package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

var (
	input_file       string
	output_file      string
	modifiers_file   string
	threads          int
	verbose          bool
	modifiers        = []string{}
	total_requests   int64
	total_failures   int64
	total_successes  int64
	print_lock       sync.Mutex
)

func init() {
	flag.StringVar(&input_file, "i", "", "Input file containing wordlist")
	flag.StringVar(&output_file, "o", "", "Output file for results (optional)")
	flag.StringVar(&modifiers_file, "m", "", "Modifiers file containing modifier list (optional)")
	flag.IntVar(&threads, "t", 10, "Number of concurrent threads")
	flag.BoolVar(&verbose, "v", false, "Enable verbose mode")
}

func print_usage() {
	fmt.Println("Usage:")
	fmt.Println("  -i <input file> : Input file containing wordlist (required)")
	fmt.Println("  -o <output file> : Output file for results (optional)")
	fmt.Println("  -m <modifiers file> : Modifiers file containing modifier list (optional)")
	fmt.Println("  -t <threads> : Number of concurrent threads (default: 10)")
	fmt.Println("Example:")
	fmt.Println("  ./s3scanner -i input.txt -o results.txt -t 20")
}

func check_url(url string, ch chan string, wg *sync.WaitGroup, verbose bool) {
	defer wg.Done()
	resp, err := http.Get(url)
	if err != nil {
		atomic.AddInt64(&total_failures, 1)
		if verbose {
			print_lock.Lock()
			fmt.Printf("\033[31m[-] %s\033[0m\n", url)
			print_lock.Unlock()
		}
		return
	}
	defer resp.Body.Close()

	atomic.AddInt64(&total_requests, 1)

	if resp.StatusCode == http.StatusOK {
		atomic.AddInt64(&total_successes, 1)
		bucket_name := strings.Split(url, ".")[0][8:]
		ch <- bucket_name
	} else {
		atomic.AddInt64(&total_failures, 1)
		if verbose {
			print_lock.Lock()
			fmt.Printf("\033[31m[-] %s\033[0m\n", url)
			print_lock.Unlock()
		}
	}
}

func get_current_open_files() (int, error) {
	data, err := os.ReadFile("/proc/sys/fs/file-nr")
	if err != nil {
		return 0, err
	}
	parts := strings.Fields(string(data))
	if len(parts) < 1 {
		return 0, fmt.Errorf("unexpected content in /proc/sys/fs/file-nr")
	}
	open_files, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	return open_files, nil
}

func display_stats(total_urls int, sem chan struct{}, stop chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var rlimit unix.Rlimit
	err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		log.Fatalf("Error getting file descriptor limit: %v", err)
	}

	max_open_files := int(rlimit.Cur)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			current_open_files, err := get_current_open_files()
			if err != nil {
				log.Printf("Error getting current open files: %v", err)
				continue
			}
			current_threads := threads - len(sem)
			progress := float64(atomic.LoadInt64(&total_requests)) / float64(total_urls) * 100

			print_lock.Lock()
			fmt.Printf("\r\033[KTotal requests: %d | Total failures: %d | Total successes: %d | Current threads: %d | Max threads: %d | Current open files: %d | Max open files: %d | Progress: %.2f%%",
				atomic.LoadInt64(&total_requests),
				atomic.LoadInt64(&total_failures),
				atomic.LoadInt64(&total_successes),
				current_threads,
				threads,
				current_open_files,
				max_open_files,
				progress)
			print_lock.Unlock()
		}
	}
}

func load_modifiers() {
	if modifiers_file != "" {
		file, err := os.Open(modifiers_file)
		if err != nil {
			log.Fatalf("Failed to open modifiers file: %v", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			modifiers = append(modifiers, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			log.Fatalf("Error reading modifiers file: %v", err)
		}
	} else {
		modifiers      = []string{
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
		}
	}
}

func main() {
	flag.Parse()

	if input_file == "" {
		print_usage()
		log.Fatal("Missing required input file")
	}

	if output_file == "" {
		output_file = fmt.Sprintf("output-%d.txt", time.Now().Unix())
	}

	load_modifiers()

	file, err := os.Open(input_file)
	if err != nil {
		log.Fatalf("Failed to open input file: %v", err)
	}
	defer file.Close()

	out_file, err := os.Create(output_file)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer out_file.Close()

	scanner := bufio.NewScanner(file)
	urls := make([]string, 0)
	for scanner.Scan() {
		word := scanner.Text()
		urls = append(urls, fmt.Sprintf("https://%s.s3.amazonaws.com/?uploads=", word))
		for _, mod := range modifiers {
			urls = append(urls, fmt.Sprintf("https://%s-%s.s3.amazonaws.com/?uploads=", mod, word))
			urls = append(urls, fmt.Sprintf("https://%s%s.s3.amazonaws.com/?uploads=", mod, word))
			urls = append(urls, fmt.Sprintf("https://%s-%s.s3.amazonaws.com/?uploads=", word, mod))
			urls = append(urls, fmt.Sprintf("https://%s%s.s3.amazonaws.com/?uploads=", word, mod))
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input file: %v", err)
	}

	var rlimit unix.Rlimit
	err = unix.Getrlimit(unix.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		log.Fatalf("Error getting file descriptor limit: %v", err)
	}

	max_threads := int(rlimit.Cur) - 10
	if threads > max_threads {
		threads = max_threads
	}

	ch := make(chan string, len(urls))
	stop := make(chan struct{})
	var wg sync.WaitGroup

	sem := make(chan struct{}, threads)

	go display_stats(len(urls), sem, stop)

	for _, url := range urls {
		wg.Add(1)
		sem <- struct{}{}
		go func(url string) {
			defer func() { <-sem }()
			check_url(url, ch, &wg, verbose)
		}(url)
	}

	go func() {
		wg.Wait()
		close(ch)
		close(stop)
	}()

	for bucket_name := range ch {
		print_lock.Lock()
		fmt.Printf("\r\033[K[+] %s\n", bucket_name)
		print_lock.Unlock()
		out_file.WriteString(bucket_name + "\n")
	}
	fmt.Printf("\r\033[K")
}
