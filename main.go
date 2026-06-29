package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	requestTimeout               = 10 * time.Second
	scannerBufferSize            = 1024 * 1024
	maxWorkerCount               = 1024
	singleModifierCandidateForms = 6
	envRegionCandidateForms      = 4
)

var (
	inputFile      string
	outputFile     string
	modifiersFile  string
	threads        int
	verbose        bool
	totalRequests  int64
	totalFailures  int64
	totalSuccesses int64
	printLock      sync.Mutex
)

type candidate struct {
	BucketName string
	URL        string
}

type candidateOptions struct {
	modifiers         []string
	comboEnvironments []string
	comboRegions      []string
}

func init() {
	flag.StringVar(&inputFile, "i", "", "Input file containing wordlist")
	flag.StringVar(&outputFile, "o", "", "Output file for results (optional)")
	flag.StringVar(&modifiersFile, "m", "", "Modifiers file containing modifier list (optional)")
	flag.StringVar(&modifiersFile, "modifiers", "", "Modifiers file containing modifier list (optional, alias for -m)")
	flag.IntVar(&threads, "t", 10, "Number of concurrent threads")
	flag.BoolVar(&verbose, "v", false, "Enable verbose mode")
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  -i <input file> : Input file containing wordlist (required)")
	fmt.Println("  -o <output file> : Output file for results (optional)")
	fmt.Println("  -m <modifiers file> : Modifiers file containing modifier list (optional)")
	fmt.Println("  -modifiers <modifiers file> : Alias for -m")
	fmt.Println("  -t <threads> : Number of concurrent threads (default: 10)")
	fmt.Println("Example:")
	fmt.Println("  ./s3scanner -i input.txt -o results.txt -t 20")
}

func defaultModifiers() []string {
	return dedupeStrings([]string{
		"prod", "dev", "qa", "uat", "bucket", "files", "archives", "backup", "backups", "cdn", "test", "stage",
		"staging", "temp", "temporary", "public", "private", "media", "data", "logs", "images", "assets", "resources",
		"docs", "documents", "reports", "analytics", "static", "content", "uploads", "downloads", "scripts", "configs",
		"configurations", "settings", "release", "releases", "home", "app", "apps", "application", "applications",
		"code", "source", "sources", "library", "libraries", "repo", "repos", "repository", "repositories", "env",
		"environment", "environments", "db", "database", "databases", "cache", "caches", "archive", "archives", "backup",
		"backups", "cdn", "proxy", "proxies", "service", "services", "api", "apis", "v1", "v2", "v3", "main", "mainnet",
		"testnet", "development", "production", "integration", "live", "snapshot", "snapshots", "audit", "audits", "log",
		"logs", "metrics", "metric", "tracking", "tracker", "tracers", "trace", "traces", "user", "users", "account",
		"accounts", "session", "sessions", "activity", "activities", "event", "events", "transaction", "transactions",
		"billing", "invoice", "invoices", "customer", "customers", "client", "clients", "partner", "partners", "vendor",
		"vendors", "supplier", "suppliers", "inventory", "inventories", "order", "orders", "purchase", "purchases",
		"sale", "sales", "discount", "discounts", "coupon", "coupons", "offer", "offers", "deal", "deals", "promo",
		"promos", "promotion", "promotions",
		"prd", "preprod", "pre-prod", "nonprod", "non-prod", "sbx", "sandbox", "demo", "beta", "alpha", "perf",
		"performance", "load", "stress", "e2e", "int", "internal", "external", "s3", "aws", "cloud", "cloudfront",
		"cf", "origin", "edge", "terraform", "tf", "tfstate", "terraform-state", "state", "remote-state", "artifact",
		"artifacts", "build", "builds", "dist", "package", "packages", "deploy", "deploys", "deployment", "deployments",
		"installer", "installers", "dump", "dumps", "export", "exports", "import", "imports", "migration", "migrations",
		"web", "website", "site", "portal", "admin", "console", "frontend", "backend", "backend-assets", "lambda",
		"ecs", "eks", "k8s", "helm", "docker", "container", "containers", "cloudtrail", "trail", "trails", "access-logs",
		"elb-logs", "alb-logs", "waf-logs", "athena", "athena-results", "query-results", "secure", "security",
		"compliance", "pii", "vault", "secrets", "keys", "us", "us-east-1", "us-west-2", "eu-west-1",
		"stg", "pre", "preview", "review", "canary", "blue", "green", "dr", "recovery", "restore", "shared",
		"common", "global", "central", "core", "platform", "infra", "infrastructure", "ops", "operations",
		"engineering", "eng", "accesslogs", "server-access-logs", "s3-logs", "s3-access-logs", "cloudwatch",
		"cwlogs", "vpc-flow-logs", "flowlogs", "guardduty", "securityhub", "aws-config", "config", "firehose",
		"kinesis-firehose", "cloudformation", "cfn", "sam", "serverless", "codepipeline", "codebuild", "codedeploy",
		"glue", "emr", "redshift", "rds", "dynamodb", "kinesis", "sagemaker", "quicksight", "ecr", "lake",
		"datalake", "data-lake", "warehouse", "lakehouse", "raw", "curated", "processed", "bronze", "silver",
		"gold", "landing", "ingest", "ingestion", "etl", "elt", "pipeline", "pipelines", "parquet", "csv", "json",
		"us-east-2", "us-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-north-1", "ap-south-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ap-northeast-2", "ca-central-1", "sa-east-1",
		"use1", "use2", "usw1", "usw2", "euw1", "euw2", "euc1", "aps1", "apse1", "apse2", "apne1",
	})
}

func defaultComboEnvironments() []string {
	return dedupeStrings([]string{
		"prod", "prd", "production", "dev", "development", "qa", "test", "stage", "staging", "stg", "uat", "pre",
		"preprod", "pre-prod", "nonprod", "non-prod", "sandbox", "sbx", "demo", "beta", "alpha", "perf",
		"performance",
	})
}

func defaultComboRegions() []string {
	return dedupeStrings([]string{
		"us", "us-east-1", "us-east-2", "us-west-1", "us-west-2", "eu-west-1", "eu-west-2", "eu-west-3",
		"eu-central-1", "eu-north-1", "ap-south-1", "ap-southeast-1", "ap-southeast-2", "ap-northeast-1",
		"ap-northeast-2", "ca-central-1", "sa-east-1", "use1", "use2", "usw1", "usw2", "euw1", "euw2", "euc1",
		"aps1", "apse1", "apse2", "apne1",
	})
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	deduped := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		deduped = append(deduped, value)
	}
	return deduped
}

func loadModifiers(path string) ([]string, error) {
	if path == "" {
		return defaultModifiers(), nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open modifiers file: %w", err)
	}
	defer file.Close()

	scanner := newLineScanner(file)
	modifiers := make([]string, 0)
	for scanner.Scan() {
		modifiers = append(modifiers, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading modifiers file: %w", err)
	}
	return dedupeStrings(modifiers), nil
}

func loadCandidateOptions(path string) (candidateOptions, error) {
	modifiers, err := loadModifiers(path)
	if err != nil {
		return candidateOptions{}, err
	}

	options := candidateOptions{modifiers: modifiers}
	if path == "" {
		options.comboEnvironments = defaultComboEnvironments()
		options.comboRegions = defaultComboRegions()
	}
	return options, nil
}

func buildBucketURL(bucketName string) string {
	return fmt.Sprintf("https://%s.s3.amazonaws.com/?uploads=", bucketName)
}

func generateCandidates(word string, modifiers []string) []candidate {
	return generateCandidatesWithOptions(word, candidateOptions{modifiers: modifiers})
}

func generateCandidatesWithOptions(word string, options candidateOptions) []candidate {
	word = strings.TrimSpace(word)
	if word == "" {
		return nil
	}

	candidates := []candidate{{BucketName: word, URL: buildBucketURL(word)}}
	for _, mod := range options.modifiers {
		for _, bucketName := range modifierBucketNames(word, mod) {
			candidates = append(candidates, candidate{
				BucketName: bucketName,
				URL:        buildBucketURL(bucketName),
			})
		}
	}
	for _, env := range options.comboEnvironments {
		for _, region := range options.comboRegions {
			for _, bucketName := range envRegionBucketNames(word, env, region) {
				candidates = append(candidates, candidate{
					BucketName: bucketName,
					URL:        buildBucketURL(bucketName),
				})
			}
		}
	}
	return candidates
}

func modifierBucketNames(word, mod string) []string {
	return []string{
		fmt.Sprintf("%s-%s", mod, word),
		fmt.Sprintf("%s%s", mod, word),
		fmt.Sprintf("%s.%s", mod, word),
		fmt.Sprintf("%s-%s", word, mod),
		fmt.Sprintf("%s%s", word, mod),
		fmt.Sprintf("%s.%s", word, mod),
	}
}

func envRegionBucketNames(word, env, region string) []string {
	return []string{
		fmt.Sprintf("%s-%s-%s", word, env, region),
		fmt.Sprintf("%s-%s-%s", env, word, region),
		fmt.Sprintf("%s.%s.%s", word, env, region),
		fmt.Sprintf("%s.%s.%s", env, word, region),
	}
}

func candidatesPerWord(modifiers []string) int {
	return 1 + len(modifiers)*singleModifierCandidateForms
}

func candidatesPerWordWithOptions(options candidateOptions) int {
	return 1 +
		len(options.modifiers)*singleModifierCandidateForms +
		len(options.comboEnvironments)*len(options.comboRegions)*envRegionCandidateForms
}

func newLineScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024), scannerBufferSize)
	return scanner
}

func countWords(file *os.File) (int, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	scanner := newLineScanner(file)
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	_, err := file.Seek(0, io.SeekStart)
	return count, err
}

func parseOpenFileCount(value string) (int, error) {
	count, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, err
	}
	maxInt := int(^uint(0) >> 1)
	if count > uint64(maxInt) {
		return maxInt, nil
	}
	return int(count), nil
}

func maxOpenFiles() (int, bool, error) {
	if runtime.GOOS != "linux" {
		return 0, false, nil
	}

	data, err := os.ReadFile("/proc/self/limits")
	if err != nil {
		return 0, true, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Max open files") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 4 {
			return 0, true, fmt.Errorf("unexpected content in /proc/self/limits")
		}
		if parts[3] == "unlimited" {
			return int(^uint(0) >> 1), true, nil
		}
		maxFiles, err := parseOpenFileCount(parts[3])
		if err != nil {
			return 0, true, err
		}
		return maxFiles, true, nil
	}
	return 0, true, fmt.Errorf("max open files limit not found in /proc/self/limits")
}

func currentOpenFiles() (int, bool, error) {
	if runtime.GOOS != "linux" {
		return 0, false, nil
	}

	data, err := os.ReadFile("/proc/sys/fs/file-nr")
	if err != nil {
		return 0, true, err
	}
	parts := strings.Fields(string(data))
	if len(parts) < 1 {
		return 0, true, fmt.Errorf("unexpected content in /proc/sys/fs/file-nr")
	}
	openFiles, err := parseOpenFileCount(parts[0])
	if err != nil {
		return 0, true, err
	}
	return openFiles, true, nil
}

func normalizeThreadCount(requested int) (int, error) {
	if requested < 1 {
		return 0, fmt.Errorf("thread count must be at least 1")
	}
	if requested > maxWorkerCount {
		requested = maxWorkerCount
	}

	maxFiles, supported, err := maxOpenFiles()
	if err != nil || !supported {
		return requested, nil
	}

	maxThreads := maxFiles - 10
	if maxThreads < 1 {
		maxThreads = 1
	}
	if requested > maxThreads {
		return maxThreads, nil
	}
	return requested, nil
}

func produceCandidates(ctx context.Context, file *os.File, options candidateOptions, jobs chan<- candidate) error {
	scanner := newLineScanner(file)
	for scanner.Scan() {
		for _, candidate := range generateCandidatesWithOptions(scanner.Text(), options) {
			select {
			case jobs <- candidate:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return scanner.Err()
}

func checkCandidate(ctx context.Context, client *http.Client, candidate candidate, verbose bool) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.URL, nil)
	if err != nil {
		atomic.AddInt64(&totalRequests, 1)
		atomic.AddInt64(&totalFailures, 1)
		printFailure(candidate.URL, verbose)
		return false
	}

	resp, err := client.Do(req)
	atomic.AddInt64(&totalRequests, 1)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			atomic.AddInt64(&totalFailures, 1)
			printFailure(candidate.URL, verbose)
		}
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		atomic.AddInt64(&totalSuccesses, 1)
		return true
	}

	atomic.AddInt64(&totalFailures, 1)
	printFailure(candidate.URL, verbose)
	return false
}

func printFailure(url string, verbose bool) {
	if !verbose {
		return
	}
	printLock.Lock()
	defer printLock.Unlock()
	fmt.Printf("\033[31m[-] %s\033[0m\n", url)
}

func writeFinding(w io.Writer, bucketName string) error {
	_, err := w.Write([]byte(bucketName + "\n"))
	return err
}

func consumeResults(results <-chan string, out io.Writer, cancel context.CancelFunc) error {
	var writeErr error
	for bucketName := range results {
		printLock.Lock()
		fmt.Printf("\r\033[K[+] %s\n", bucketName)
		printLock.Unlock()
		if err := writeFinding(out, bucketName); err != nil && writeErr == nil {
			writeErr = fmt.Errorf("failed to write result: %w", err)
			cancel()
		}
	}
	return writeErr
}

func displayStats(totalCandidates int, maxThreads int, activeWorkers *int64, stop <-chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	maxOpenFilesText := "n/a"
	if maxFiles, supported, err := maxOpenFiles(); err == nil && supported {
		maxOpenFilesText = strconv.Itoa(maxFiles)
	}

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			currentOpenFilesText := "n/a"
			if currentFiles, supported, err := currentOpenFiles(); err == nil && supported {
				currentOpenFilesText = strconv.Itoa(currentFiles)
			}
			progress := progressPercent(totalCandidates)

			printLock.Lock()
			fmt.Printf("\r\033[KTotal requests: %d | Total failures: %d | Total successes: %d | Current threads: %d | Max threads: %d | Current open files: %s | Max open files: %s | Progress: %.2f%%",
				atomic.LoadInt64(&totalRequests),
				atomic.LoadInt64(&totalFailures),
				atomic.LoadInt64(&totalSuccesses),
				atomic.LoadInt64(activeWorkers),
				maxThreads,
				currentOpenFilesText,
				maxOpenFilesText,
				progress)
			printLock.Unlock()
		}
	}
}

func progressPercent(totalCandidates int) float64 {
	if totalCandidates <= 0 {
		return 100
	}
	return float64(atomic.LoadInt64(&totalRequests)) / float64(totalCandidates) * 100
}

func runScan(inputPath, outputPath, modifiersPath string, requestedThreads int, verbose bool, client *http.Client) error {
	resetScanStats()

	if inputPath == "" {
		printUsage()
		return fmt.Errorf("missing required input file")
	}
	if outputPath == "" {
		outputPath = fmt.Sprintf("output-%d.txt", time.Now().Unix())
	}
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	options, err := loadCandidateOptions(modifiersPath)
	if err != nil {
		return err
	}

	workerCount, err := normalizeThreadCount(requestedThreads)
	if err != nil {
		return err
	}

	file, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	wordCount, err := countWords(file)
	if err != nil {
		return fmt.Errorf("failed to count input words: %w", err)
	}
	totalCandidates := wordCount * candidatesPerWordWithOptions(options)

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}

	jobs := make(chan candidate, workerCount*2)
	results := make(chan string, workerCount)
	stopStats := make(chan struct{})
	producerErr := make(chan error, 1)
	var wg sync.WaitGroup
	var activeWorkers int64

	go displayStats(totalCandidates, workerCount, &activeWorkers, stopStats)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case candidate, ok := <-jobs:
					if !ok {
						return
					}
					atomic.AddInt64(&activeWorkers, 1)
					if checkCandidate(ctx, client, candidate, verbose) {
						select {
						case results <- candidate.BucketName:
						case <-ctx.Done():
						}
					}
					atomic.AddInt64(&activeWorkers, -1)
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		producerErr <- produceCandidates(ctx, file, options, jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
		close(stopStats)
	}()

	writeErr := consumeResults(results, outFile, cancel)

	closeErr := outFile.Close()
	fmt.Printf("\r\033[K")

	if err := <-producerErr; err != nil && writeErr == nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("error reading input file: %w", err)
	}
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close output file: %w", closeErr)
	}
	return nil
}

func resetScanStats() {
	atomic.StoreInt64(&totalRequests, 0)
	atomic.StoreInt64(&totalFailures, 0)
	atomic.StoreInt64(&totalSuccesses, 0)
}

func main() {
	flag.Parse()
	if err := runScan(inputFile, outputFile, modifiersFile, threads, verbose, nil); err != nil {
		log.Fatal(err)
	}
}
