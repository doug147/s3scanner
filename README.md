# S3 Bucket Scanner

This Go program is designed to scan S3 bucket URLs to check their availability and determine if they are public. It generates potential S3 bucket names based on a provided wordlist and optional modifiers, then checks if those buckets exist.

The program runs multithreaded HTTP requests and reports the results, including total requests, successes, failures, and progress during execution.

## Features

- Multithreaded scanning of S3 buckets
- Custom wordlists for bucket names
- Dynamic modifier input through an optional file
- Automatically determines the output file if not provided
- Detailed progress and statistics reporting
- Adjustable concurrency (threads)
- Verbose mode for more detailed logs

## Table of Contents

- [Installation](#installation)
- [Usage](#usage)
- [Parameters](#parameters)
- [Examples](#examples)

## Installation

1. Ensure you have Go installed on your machine. You can download it from [Go's official site](https://golang.org/dl/).
2. Clone this repository:
   ```
   git clone https://github.com/doug147/s3scanner.git
   cd s3scanner
   ```
3. Build the Go program:
   ```
   go build -o s3scanner .
   ```

## Usage

The program takes a wordlist of potential bucket names as input, generates potential S3 bucket URLs, and checks their availability.

Basic command:
```
./s3scanner -i <input-file> [-o <output-file>] [-t <threads>] [-v] [-modifiers <modifiers-file>]
```

If no output file is specified, the results will be saved in a file named `output-<unix_epoch_time>.txt`.

### Parameters

| Parameter        | Required | Description                                                                                                                                   |
|------------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| `-i`             | Yes      | Input file containing a wordlist (one word per line). These words will be used to generate potential S3 bucket names.                         |
| `-o`             | No       | Output file for saving results. If not specified, the program will save the results to a file named `output-<unix_epoch_time>.txt`.           |
| `-t`             | No       | Number of concurrent threads to use for scanning (default: 10). The program automatically adjusts if the specified value exceeds the system’s file descriptor limit. |
| `-v`             | No       | Enable verbose mode. In verbose mode, each failed request is printed in the terminal.                                                         |
| `-modifiers`     | No       | An optional modifiers file containing additional modifiers for generating more complex S3 bucket names. These should be listed one per line. If not specified, the program uses a predefined list of common modifiers (e.g., `prod`, `dev`, `test`, `stage`, etc.). |

### Output

The program saves the names of publicly accessible S3 buckets in the specified output file or a file named `output-<unix_epoch_time>.txt` by default. It only prints the bucket name to the console and the file, omitting the full S3 URL.

### Example

#### Basic Example
```
./s3scanner -i wordlist.txt -o results.txt
```

- Scans using the words from `wordlist.txt` and saves the results to `results.txt`.

#### Verbose Mode
```
./s3scanner -i wordlist.txt -t 20 -v
```

- Runs the scanner with 20 threads, enabling verbose logging to show all failed and successful requests in the terminal.

#### Using a Modifiers File
```
./s3scanner -i wordlist.txt -modifiers mods.txt -t 15
```

- Scans using the words from `wordlist.txt`, appending modifiers from `mods.txt` to generate more potential bucket names.

#### Output File with Unix Epoch
```
./s3scanner -i wordlist.txt
```

- Automatically generates an output file with a name like `output-1725562640.txt`, where `1725562640` is the Unix epoch timestamp at the time the program runs.

## Statistics and Progress

During the scanning process, the program prints detailed statistics, including:

- Total requests made
- Total successes (i.e., publicly accessible S3 buckets)
- Total failures
- Number of active threads
- Current number of open file descriptors
- Overall progress percentage
