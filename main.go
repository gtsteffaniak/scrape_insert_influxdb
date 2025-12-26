package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"scrape/docker"
	"scrape/query"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DATABASE_URL         string
	GET_REQUEST_TARGET   string
	SLEEP_TIME           int
	DB_ATTRIBUTE_NAME    string
	RECORD_EMPTY_OR_ZERO bool
	FIELDS               map[string]string
	IS_DOCKER_STATS      bool
	DOCKER_ENDPOINT      string
}

type YAMLConfig struct {
	Global struct {
		DatabaseURL string `yaml:"database_url"`
	} `yaml:"global"`
	Insert map[string]struct {
		URL            string            `yaml:"url"`
		WaitTime       int               `yaml:"waitTime"`
		StoreBlank     bool              `yaml:"storeBlank"`
		DatabaseURL    string            `yaml:"databaseUrl"`
		Fields         map[string]string `yaml:"fields"`
		DockerStats    bool              `yaml:"dockerStats"`
		DockerEndpoint string            `yaml:"dockerEndpoint"`
	} `yaml:"insert"`
}

func main() {
	fmt.Println("Starting...")

	configs, err := loadConfigsFromYAML("config.yaml")
	if err != nil {
		log.Fatalf("Error loading YAML config: %v", err)
	}

	if len(configs) == 0 {
		fmt.Println("No valid configs found.")
		return
	}

	for _, config := range configs {
		if config.IS_DOCKER_STATS {
			go func(cfg Config) {
				docker.StatsCollector(cfg.DB_ATTRIBUTE_NAME, cfg.SLEEP_TIME, cfg.RECORD_EMPTY_OR_ZERO, func(payload string) {
					log.Printf("INSERT : [%s]", payload)
					if err := postDataToInfluxDB(cfg.DATABASE_URL, payload); err != nil {
						log.Printf("[%s] Failed to post Docker stats data: %v", cfg.DB_ATTRIBUTE_NAME, err)
					}
				})
			}(config)
		} else {
			go jsonChecker(config)
		}
	}

	select {}
}

func loadConfigsFromYAML(path string) ([]Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open YAML file: %v", err)
	}
	defer file.Close()

	var yconf YAMLConfig
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&yconf); err != nil {
		return nil, fmt.Errorf("failed to decode YAML: %v", err)
	}

	if yconf.Global.DatabaseURL == "" {
		return nil, fmt.Errorf("global.database_url must be specified")
	}

	var configs []Config
	for name, entry := range yconf.Insert {
		if entry.DockerStats {
			// Docker stats configuration
			if entry.WaitTime <= 0 {
				log.Printf("[%s] Skipping invalid Docker stats config - invalid wait time", name)
				continue
			}
			db := entry.DatabaseURL
			if db == "" {
				db = yconf.Global.DatabaseURL
			}
			dockerEndpoint := entry.DockerEndpoint
			if dockerEndpoint == "" {
				dockerEndpoint = "unix:///var/run/docker.sock"
			}
			config := Config{
				DATABASE_URL:         db,
				DB_ATTRIBUTE_NAME:    name,
				SLEEP_TIME:           entry.WaitTime,
				RECORD_EMPTY_OR_ZERO: entry.StoreBlank,
				IS_DOCKER_STATS:      true,
				DOCKER_ENDPOINT:      dockerEndpoint,
			}
			config.printValues()
			configs = append(configs, config)
		} else {
			// Regular HTTP API configuration
			if entry.URL == "" || entry.WaitTime <= 0 {
				log.Printf("[%s] Skipping invalid YAML config", name)
				continue
			}
			if len(entry.Fields) == 0 {
				log.Printf("[%s] Skipping config, no fields specified", name)
				continue
			}
			db := entry.DatabaseURL
			if db == "" {
				db = yconf.Global.DatabaseURL
			}
			config := Config{
				DATABASE_URL:         db,
				DB_ATTRIBUTE_NAME:    name,
				GET_REQUEST_TARGET:   entry.URL,
				SLEEP_TIME:           entry.WaitTime,
				RECORD_EMPTY_OR_ZERO: entry.StoreBlank,
				FIELDS:               entry.Fields,
				IS_DOCKER_STATS:      false,
			}
			config.printValues()
			configs = append(configs, config)
		}
	}

	return configs, nil
}

func jsonChecker(config Config) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 3 * time.Second,
	}

	firstRun := true

	for {
		if !firstRun {
			time.Sleep(time.Duration(config.SLEEP_TIME) * time.Second)
		}
		firstRun = false

		resp, err := client.Get(config.GET_REQUEST_TARGET)
		if err != nil {
			log.Printf("[%s] Failed to fetch data : %v", config.DB_ATTRIBUTE_NAME, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("[%s] Failed to read response body - %v", config.DB_ATTRIBUTE_NAME, err)
			continue
		}

		var data interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			log.Printf("[%s] Failed to parse JSON response : %v", config.DB_ATTRIBUTE_NAME, err)
			continue
		}

		fields := make(map[string]string)
		for fieldName, jq := range config.FIELDS {
			val := query.ExtractValueUsingJSONQuery(data, jq)
			if !config.RECORD_EMPTY_OR_ZERO && (val == "" || val == "0") {
				log.Printf("[%s] Skipping field [%s] with empty or zero value", config.DB_ATTRIBUTE_NAME, fieldName)
				continue
			}
			fields[fieldName] = val
		}

		if len(fields) == 0 {
			log.Printf("[%s] No valid fields to insert", config.DB_ATTRIBUTE_NAME)
			continue
		}

		payload := config.DB_ATTRIBUTE_NAME + " "
		for key, val := range fields {
			payload += formatField(sanitize(key), val) + ","
		}
		payload = strings.TrimSuffix(payload, ",")
		log.Printf("INSERT : [%s]", payload)
		if err := postDataToInfluxDB(config.DATABASE_URL, payload); err != nil {
			log.Printf("[%s] Failed to post data : %v", config.DB_ATTRIBUTE_NAME, err)
		}
	}
}

func formatField(name string, value interface{}) string {
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("%s=%g", name, v)
	case int:
		return fmt.Sprintf("%s=%d", name, v)
	case string:
		if _, err := strconv.ParseFloat(v, 64); err == nil {
			return fmt.Sprintf("%s=%s", name, v)
		}
		return fmt.Sprintf(`%s="%s"`, name, escapeQuotes(v))
	default:
		// fallback to quoted string
		str := fmt.Sprintf("%v", value)
		return fmt.Sprintf(`%s="%s"`, name, escapeQuotes(str))
	}
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func sanitize(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}

// readTokenFromFile reads a token from a file path
func readTokenFromFile(filePath string) (string, error) {
	if filePath == "" {
		return "", nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read token file %s: %v", filePath, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// getToken retrieves the InfluxDB token from environment variable or file
func getToken() (string, error) {
	// First, try direct environment variable
	token := os.Getenv("INFLUXDB_TOKEN")
	if token != "" {
		return strings.TrimSpace(token), nil
	}

	// Fall back to token file if environment variable is not set
	tokenFile := os.Getenv("INFLUXDB_TOKEN_FILE")
	if tokenFile != "" {
		token, err := readTokenFromFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("failed to read token from file: %v", err)
		}
		if token == "" {
			return "", fmt.Errorf("token file is empty")
		}
		return token, nil
	}

	return "", fmt.Errorf("neither INFLUXDB_TOKEN nor INFLUXDB_TOKEN_FILE is set")
}

// postDataToInfluxDB posts data to InfluxDB, supporting both 1.x and 2.x versions
func postDataToInfluxDB(url, payload string) error {
	// Check for InfluxDB 2.0+ environment variables
	org := os.Getenv("INFLUXDB_ORG")
	bucket := os.Getenv("INFLUXDB_BUCKET")

	var req *http.Request
	var err error

	// If InfluxDB 2.0+ variables are set, use v2 API
	if org != "" && bucket != "" {
		token, err := getToken()
		if err != nil {
			return fmt.Errorf("failed to get token: %v", err)
		}

		// Construct InfluxDB 2.0 write URL
		// Remove any existing path/query from base URL
		baseURL := strings.TrimSuffix(url, "/")
		if strings.Contains(baseURL, "/write") {
			// Extract base URL (e.g., http://influxdb:8086 from http://influxdb:8086/write?db=home)
			parts := strings.Split(baseURL, "/write")
			baseURL = parts[0]
		}
		v2URL := fmt.Sprintf("%s/api/v2/write?org=%s&bucket=%s", baseURL, org, bucket)

		req, err = http.NewRequest("POST", v2URL, bytes.NewBufferString(payload))
		if err != nil {
			return fmt.Errorf("failed to create request: %v", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Token %s", token))
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		// Use InfluxDB 1.x format (backward compatibility)
		req, err = http.NewRequest("POST", url, bytes.NewBufferString(payload))
		if err != nil {
			return fmt.Errorf("failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post error: %v", err)
	}
	defer resp.Body.Close()

	// InfluxDB 2.0 returns 204 on success, 1.x also returns 204
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("non-204 response: %d, body: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *Config) printValues() {
	if c.IS_DOCKER_STATS {
		log.Printf("DOCKER_STATS              : [%s] %t", c.DB_ATTRIBUTE_NAME, c.IS_DOCKER_STATS)
		log.Printf("DOCKER_ENDPOINT           : [%s] %s", c.DB_ATTRIBUTE_NAME, c.DOCKER_ENDPOINT)
		log.Printf("SLEEP_TIME                : [%s] %d", c.DB_ATTRIBUTE_NAME, c.SLEEP_TIME)
		log.Printf("RECORD_EMPTY_OR_ZERO      : [%s] %t", c.DB_ATTRIBUTE_NAME, c.RECORD_EMPTY_OR_ZERO)
	} else {
		log.Printf("GET_REQUEST_TARGET        : [%s] %s", c.DB_ATTRIBUTE_NAME, c.GET_REQUEST_TARGET)
		log.Printf("JSON_QUERY                : [%s] %s", c.DB_ATTRIBUTE_NAME, c.FIELDS)
		log.Printf("SLEEP_TIME                : [%s] %d", c.DB_ATTRIBUTE_NAME, c.SLEEP_TIME)
		log.Printf("RECORD_EMPTY_OR_ZERO      : [%s] %t", c.DB_ATTRIBUTE_NAME, c.RECORD_EMPTY_OR_ZERO)
	}
	log.Print("==============================")
}
