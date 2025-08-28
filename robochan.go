package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Result struct {
	URL          string   `json:"url"`
	RobotsURL    string   `json:"robots_url"`
	Content      string   `json:"content"`
	GeneratedURLs []string `json:"generated_urls,omitempty"`
	Error        string   `json:"error,omitempty"`
}

var (
	generateLinks bool
	noRedirect    bool
	outputFile    string
	outputFormat  string
	results       []Result
)

func main() {
	// Parse command line flags
	flag.BoolVar(&generateLinks, "g", false, "Generate full URLs from robots.txt paths")
	flag.BoolVar(&noRedirect, "n", false, "Do not follow redirects")
	flag.StringVar(&outputFile, "o", "", "Output file for generated links (e.g., output.txt or output.json)")
	flag.Parse()

	// Determine output format from file extension
	if outputFile != "" {
		ext := strings.ToLower(filepath.Ext(outputFile))
		if ext == ".json" {
			outputFormat = "json"
		} else {
			outputFormat = "txt"
		}
	}

	// Print banner
	printBanner()

	scanner := bufio.NewScanner(os.Stdin)
	var urls []string

	// Read URLs from stdin
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			urls = append(urls, line)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Error reading input: %v\n", err)
		os.Exit(1)
	}

	if len(urls) == 0 {
		fmt.Println("[!] No URLs provided")
		os.Exit(1)
	}

	// Process URLs
	if len(urls) == 1 {
		// Single URL mode
		fmt.Println("[+] Fetching data..........")
		fmt.Println()
		fetchAndPrintRobots(urls[0], false)
	} else {
		// Multiple URLs mode
		fmt.Printf("[+] %d URLs found in input\n", len(urls))
		fmt.Println("[+] Fetching data on multiple URLs...........")
		fmt.Println()
		
		for _, u := range urls {
			fmt.Printf("[-] %s\n", u)
			fmt.Println("-------------------------------------------------------------------------")
			fmt.Println()
			fetchAndPrintRobots(u, true)
			fmt.Println()
		}
	}

	// Save results if output file is specified
	if outputFile != "" && len(results) > 0 {
		saveResults()
	}
}

func printBanner() {
	banner := `
⠀⠀⠀⠀⠀⠀⠀⠀⢰⣶⣶⣤⣄⣀⣀⣀⣤⣤⣀⣀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⢸⠙⡿⣋⣿⣿⣿⣿⣿⣿⣿⣿⣿⣶⣶⣶⠶⠀⢹
⠀⠀⠀⠀⠀⠀⠀⠀⠈⣢⣶⣧⣾⣯⣿⣿⣿⣿⣿⣿⣛⡛⣿⠥⠀⠀⢸
⠀⠀⠀⠀⠀⠀⠀⠀⣶⣿⣿⡿⣽⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣷⡄⢀⠃
⠀⠀⠀⠀⠀⠀⠀⢸⣿⣿⣿⠃⣿⣿⣿⣿⢸⣿⣿⣿⣿⣿⣿⣿⣿⣷      ___      _          ___ _              
⠀⠀⠀⠀⠀⠀⠀⢸⣿⣿⢻⠈⣿⣿⣿⣿⠸⣿⣿⣿⣿⣿⣿⣿⣿⡇     | _ \____| |__ __  _/ __| |_  __ _ _ _  
⠀⠀⠀⠀⠀⠀⠀⢸⣿⡿⠛⣳⣮⠉⠉⠉⢼⢞⣿⣿⣷⣿⣿⣿⣿      |   / _ \ '_ \/ _ \| (__| ' \/ _' | ' \ 
⠀⠀⠀⠀⠀⠀⠀⠸⣿⣿⠃⠿⠟⠀⠀⠀⠘⠬⠿⠟⣽⣿⣿⣿⡿      |_|_\___/_.__/\___/ \___|_||_\__,_|_||_|
⠀⠀⠀⠀⠀⠀⠀⠀⣻⣿⣧⠀⠀⠀⠀⠀⠀⠀⠀⢠⣿⣿⣿⣿⡇              v2.0 - robots.txt reconnaissance
⠀⡀⠀⠀⠀⠀⠀⣰⠟⣿⣿⣶⣄⣀⠐⠀⠀⡀⣔⣾⣿⣿⣿⣿⡅			      Max - Muxammil
⠀⠪⠂⡀⣠⣴⠚⡇⠀⣨⡧⠿⠿⠿⢷⢶⢿⣶⠿⠟⣿⣿⣿⣿⡇ 
`
	fmt.Println(banner)
}

func fetchAndPrintRobots(targetURL string, multiMode bool) {
	result := Result{URL: targetURL}
	
	// Parse and validate URL
	// If no scheme is provided, prepend https://
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}
	
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		fmt.Printf("[!] Invalid URL: %v\n", err)
		result.Error = fmt.Sprintf("Invalid URL: %v", err)
		results = append(results, result)
		return
	}

	// Ensure we have a scheme (fallback)
	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https"
	}

	// Build robots.txt URL
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", parsedURL.Scheme, parsedURL.Host)
	result.RobotsURL = robotsURL

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if noRedirect {
				return http.ErrUseLastResponse
			}
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Create request
	req, err := http.NewRequest("GET", robotsURL, nil)
	if err != nil {
		fmt.Printf("[!] Error creating request: %v\n", err)
		result.Error = fmt.Sprintf("Error creating request: %v", err)
		results = append(results, result)
		return
	}

	// Set realistic browser headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/plain,text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[!] Error fetching %s: %v\n", robotsURL, err)
		result.Error = fmt.Sprintf("Error fetching: %v", err)
		results = append(results, result)
		return
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode == 404 {
		fmt.Println("[!] robots.txt not found (404)")
		result.Error = "robots.txt not found (404)"
		results = append(results, result)
		return
	} else if resp.StatusCode != 200 {
		fmt.Printf("[!] Unexpected status code: %d\n", resp.StatusCode)
		result.Error = fmt.Sprintf("Unexpected status code: %d", resp.StatusCode)
		results = append(results, result)
		return
	}

	// Read and print content
	var reader io.Reader = resp.Body
	
	// Handle gzip compression
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			fmt.Printf("[!] Error creating gzip reader: %v\n", err)
			result.Error = fmt.Sprintf("Error creating gzip reader: %v", err)
			results = append(results, result)
			return
		}
		defer gzipReader.Close()
		reader = gzipReader
	}
	
	body, err := io.ReadAll(reader)
	if err != nil {
		fmt.Printf("[!] Error reading response: %v\n", err)
		result.Error = fmt.Sprintf("Error reading response: %v", err)
		results = append(results, result)
		return
	}

	content := string(body)
	result.Content = content
	
	// Print content
	if len(content) > 0 {
		fmt.Print(content)
		if !strings.HasSuffix(content, "\n") {
			fmt.Println()
		}
		
		// Generate URLs if flag is set
		if generateLinks {
			generatedURLs := extractAndGenerateURLs(content, parsedURL)
			result.GeneratedURLs = generatedURLs
			
			if len(generatedURLs) > 0 {
				fmt.Println("\n[+] Generated URLs from robots.txt paths:")
				fmt.Println("==========================================")
				for _, genURL := range generatedURLs {
					fmt.Println(genURL)
				}
			}
		}
	} else {
		fmt.Println("[!] robots.txt is empty")
		result.Error = "robots.txt is empty"
	}
	
	results = append(results, result)
}

func extractAndGenerateURLs(content string, baseURL *url.URL) []string {
	var urls []string
	seen := make(map[string]bool)
	
	// Patterns to match paths in robots.txt
	patterns := []string{
		`(?i)Allow:\s*(.+)`,
		`(?i)Disallow:\s*(.+)`,
		`(?i)Sitemap:\s*(.+)`,
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(content, -1)
		
		for _, match := range matches {
			if len(match) > 1 {
				path := strings.TrimSpace(match[1])
				
				// Skip empty paths and comments
				if path == "" || strings.HasPrefix(path, "#") {
					continue
				}
				
				// Generate full URL
				var fullURL string
				if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
					// Already a full URL (like sitemaps)
					fullURL = path
				} else {
					// Relative path - just concatenate with domain
					if !strings.HasPrefix(path, "/") {
						path = "/" + path
					}
					
					fullURL = fmt.Sprintf("%s://%s%s", baseURL.Scheme, baseURL.Host, path)
				}
				
				// Add to list if not seen
				if !seen[fullURL] {
					seen[fullURL] = true
					urls = append(urls, fullURL)
				}
			}
		}
	}
	
	return urls
}

func saveResults() {
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[!] Error creating output file: %v\n", err)
		return
	}
	defer file.Close()
	
	if outputFormat == "json" {
		// Save as JSON
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(results); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Error writing JSON: %v\n", err)
			return
		}
		fmt.Printf("\n[+] Results saved to %s (JSON format)\n", outputFile)
	} else {
		// Save as text (only generated URLs)
		writer := bufio.NewWriter(file)
		for _, result := range results {
			if len(result.GeneratedURLs) > 0 {
				writer.WriteString(fmt.Sprintf("# %s\n", result.URL))
				writer.WriteString(fmt.Sprintf("# Robots.txt: %s\n", result.RobotsURL))
				writer.WriteString("#" + strings.Repeat("-", 60) + "\n")
				for _, genURL := range result.GeneratedURLs {
					writer.WriteString(genURL + "\n")
				}
				writer.WriteString("\n")
			}
		}
		writer.Flush()
		fmt.Printf("\n[+] Generated URLs saved to %s (TXT format)\n", outputFile)
	}
}