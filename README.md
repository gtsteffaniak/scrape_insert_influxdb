# Scrape Insert InfluxDB

A lightweight application with concurrent api scraping for JSON payload REST APIs and Docker container statistics which get inserted into InfluxDB. Perfect for monitoring APIs, tracking metrics, and collecting Docker container performance data, and general api data tracking.

## Features

- **HTTP API Scraping**: Periodically fetch data from any JSON API endpoint
- **JSONPath Queries**: Extract specific fields from JSON responses using JSONPath expressions
- **Docker Stats Collection**: Monitor Docker container CPU, memory, network, and I/O statistics
- **InfluxDB Integration**: Automatically format and insert data into InfluxDB
- **Concurrent Collection**: Run multiple data collection tasks simultaneously
- **Configurable**: YAML-based configuration for easy setup
- **Multi-arch Support**: Docker images built for `linux/amd64`, `linux/arm64`, and `linux/arm/v7`

## How It Works

The application reads a `config.yaml` file that defines one or more data collection tasks. Each task runs in its own goroutine and operates independently:

1. **HTTP API Tasks**:
   - Makes GET requests to configured URLs at specified intervals
   - Parses JSON responses
   - Extracts values using JSONPath queries
   - Formats data as InfluxDB line protocol
   - Posts to InfluxDB write endpoint

2. **Docker Stats Tasks**:
   - Connects to Docker daemon via Unix socket
   - Lists all running containers
   - Collects CPU, memory, network, and I/O statistics for each container
   - Calculates percentages and metrics
   - Formats and sends to InfluxDB

## Configuration

Create a `config.yaml` file with the following structure:

```yaml
global:
  database_url: http://localhost:9086/write?db=home

insert:
  # HTTP API Example
  dockerhub_pull_count:
    url: https://hub.docker.com/v2/repositories/gtstef/filebrowser/
    waitTime: 3600  # seconds between requests
    storeBlank: false  # skip empty or zero values
    fields:
      pulls: $.pull_count  # JSONPath query
    # Optional: override global database URL
    # databaseUrl: http://other-host:9086/write?db=other

  # Multiple fields example
  gportal_status:
    url: https://gportal.link/api/health/
    waitTime: 60
    storeBlank: false
    fields:
      filebrowser: $[?(@.name=="File-Browser")].health.Status
      blog: $[?(@.name=="Blog")].health.Status
      photos: $[?(@.name=="Photos")].health.Status

  # Docker Stats Example
  docker_container_stats:
    dockerStats: true
    dockerEndpoint: unix:///var/run/docker.sock
    waitTime: 30
    storeBlank: false
```

### Configuration Fields

#### Global Settings
- `database_url` (required): Default InfluxDB write endpoint URL

#### Task Settings
- `url`: HTTP endpoint to scrape (required for HTTP tasks)
- `waitTime`: Seconds to wait between requests (required, must be > 0)
- `storeBlank`: Whether to store empty or zero values (default: false)
- `fields`: Map of field names to JSONPath queries (required for HTTP tasks)
- `databaseUrl`: Override global database URL for this task (optional)
- `dockerStats`: Enable Docker stats collection (set to `true` for Docker tasks)
- `dockerEndpoint`: Docker daemon endpoint (default: `unix:///var/run/docker.sock`)

### JSONPath Examples

The application uses JSONPath to extract values from JSON responses:

- `$.pull_count` - Simple field access
- `$.stargazers_count` - Nested field
- `$[?(@.name=="File-Browser")].health.Status` - Array filtering and field access
- `$[0].value` - Array index access

## Usage

### Local Development

1. **Build the application**:
   ```bash
   go build -o scrape .
   ```

2. **Create your `config.yaml`** (see Configuration section above)

3. **Run the application**:
   ```bash
   ./scrape
   ```

### Docker

#### Using Pre-built Images

Images are automatically built and pushed to:
- Docker Hub: `gtstef/scrape-insert-influxdb:latest`
- GitHub Container Registry: `ghcr.io/gtsteffaniak/scrape-insert-influxdb:latest`

#### Run with Docker

```bash
docker run -d \
  --name scrape-influxdb \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v $(pwd)/config.yaml:/config.yaml:ro \
  gtstef/scrape-insert-influxdb:latest
```

**Note**: The Docker socket mount (`-v /var/run/docker.sock:/var/run/docker.sock:ro`) is only needed if you're collecting Docker stats.

#### Build Locally

```bash
docker build -f dockerfile -t scrape-influxdb .
docker run -v $(pwd)/config.yaml:/config.yaml:ro scrape-influxdb
```

### Docker Compose

```yaml
services:
  scrape-influxdb:
    image: gtstef/scrape-insert-influxdb:latest
    volumes:
      - ./config.yaml:/config.yaml:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
    restart: unless-stopped
```

## InfluxDB Data Format

Data is inserted using InfluxDB line protocol:

```
measurement_name,tag=value field1=value1,field2=value2
```

### HTTP API Tasks
- **Measurement**: The task name from config (e.g., `dockerhub_pull_count`)
- **Fields**: Extracted values from JSONPath queries
- **Tags**: None (can be extended)

### Docker Stats Tasks
- **Measurement**: The task name from config (e.g., `docker_container_stats`)
- **Tag**: `container` (container name)
- **Fields**:
  - `cpu_percent`: CPU usage percentage
  - `memory_usage_mb`: Memory usage in MB (working set)
  - `memory_limit_mb`: Memory limit in MB
  - `memory_percent`: Memory usage percentage
  - `network_rx_bytes`: Network received bytes
  - `network_tx_bytes`: Network transmitted bytes
  - `block_read_bytes`: Block I/O read bytes
  - `block_write_bytes`: Block I/O write bytes

## Examples

### Example 1: Monitor GitHub Repository Stars

```yaml
global:
  database_url: http://influxdb:9086/write?db=metrics

insert:
  github_stars:
    url: https://api.github.com/repos/gtsteffaniak/filebrowser
    waitTime: 3600
    storeBlank: false
    fields:
      stars: $.stargazers_count
      forks: $.forks_count
      open_issues: $.open_issues_count
```

### Example 2: Monitor Multiple Services

```yaml
global:
  database_url: http://influxdb:9086/write?db=metrics

insert:
  service_health:
    url: https://api.example.com/health
    waitTime: 30
    storeBlank: true
    fields:
      status: $.status
      uptime: $.uptime_seconds
      version: $.version
```

### Example 3: Docker Container Monitoring

```yaml
global:
  database_url: http://influxdb:9086/write?db=metrics

insert:
  container_metrics:
    dockerStats: true
    waitTime: 15
    storeBlank: false
```
