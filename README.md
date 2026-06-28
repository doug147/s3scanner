# S3 Bucket Scanner

This Go CLI generates possible S3 bucket names from a wordlist and optional modifiers, then probes each generated bucket URL with the `?uploads=` query. A `200 OK` result means the multipart upload listing probe succeeded for that bucket endpoint; it is not a complete proof that the bucket is generally public or that objects are publicly readable.

Only scan buckets that you own or are authorized to assess.

## Features

- Concurrent S3 bucket endpoint probing
- Custom wordlists for bucket-name candidates
- Optional modifier files for expanded candidate generation
- Bounded worker pipeline instead of precomputing every URL
- Configurable concurrency
- Request timeouts to avoid stalled scans
- Progress and result output
- Cross-platform build support with Linux-only file-descriptor telemetry when available

## Installation

1. Install Go `1.25.11` or newer in the active Go 1.25 release line.
2. Clone this repository:
   ```sh
   git clone https://github.com/doug147/s3scanner.git
   cd s3scanner
   ```
3. Build the CLI:
   ```sh
   go build -o s3scanner .
   ```

On Windows, the output binary will normally be `s3scanner.exe` unless you provide a different `-o` value to `go build`.

## Usage

```sh
./s3scanner -i <input-file> [-o <output-file>] [-t <threads>] [-v] [-m <modifiers-file>]
```

The `-modifiers` flag is also accepted as an alias for `-m`.

If no output file is specified, results are saved to `output-<unix_epoch_time>.txt`.

## Parameters

| Parameter | Required | Description |
| --- | --- | --- |
| `-i` | Yes | Input wordlist, one candidate base word per line. Blank lines are ignored. |
| `-o` | No | Output file for successful probe results. Defaults to `output-<unix_epoch_time>.txt`. |
| `-m` | No | Modifier file, one modifier per line. Duplicate and blank modifiers are ignored. |
| `-modifiers` | No | Alias for `-m`. |
| `-t` | No | Number of concurrent workers. Must be between `1` and `1024`; defaults to `10`. Linux also clamps this below the process file-descriptor limit when available. |
| `-v` | No | Verbose mode. Prints failed probe URLs. |

## Examples

### Basic Scan

```sh
./s3scanner -i wordlist.txt -o results.txt
```

### Verbose Mode

```sh
./s3scanner -i wordlist.txt -t 20 -v
```

### Modifier File

```sh
./s3scanner -i wordlist.txt -m mods.txt -t 15
```

The compatibility alias works too:

```sh
./s3scanner -i wordlist.txt -modifiers mods.txt -t 15
```

## Output

Successful probe results are printed as bucket names and written one per line to the output file. The full probe URL is not written.

## Statistics and Progress

During scanning, the CLI prints:

- Completed requests
- Failed requests
- Successful probe results
- Active worker count
- Configured worker limit
- Open-file telemetry when the platform supports it
- Overall progress percentage

On non-Linux platforms, open-file telemetry is shown as `n/a`; scanning still continues.

## Verification

Recommended local checks:

```sh
gofmt -l .
go test ./...
go vet ./...
go build ./...
govulncheck ./...
```
