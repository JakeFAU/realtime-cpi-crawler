# Realtime CPI Web Crawler Instruction Manual

This manual provides comprehensive instructions on how to use, configure, and operate the Realtime CPI Web Crawler. Understanding these settings is crucial for effectively controlling the crawler's behavior, performance, and interaction with external services.

## Table of Contents

1. [Usage](#usage)
2. [Configuration](#configuration)
    * [`config.yaml` File](#configyaml-file)
    * [Environment Variables](#environment-variables)
    * [Configuration Precedence](#configuration-precedence)
3. [Configuration Settings Reference](#configuration-settings-reference)
    * [Crawler Settings](#crawler-settings)
    * [HTTP Settings](#http-settings)
    * [Storage Settings](#storage-settings)
    * [Database Settings](#database-settings)
    * [Queue Settings](#queue-settings)

## 1. Usage

The web crawler is a command-line application (CLI) built with Go and powered by Cobra. The primary command to start a crawl is `crawl`.

### Running the Crawler

To execute the crawler, navigate to the `webcrawler` directory and run the `crawl` command:

```bash
go run main.go crawl
```

This will initiate a crawl using the configuration loaded from `config.yaml` (if present) or environment variables/defaults. The crawler will log its activities to the console.

### Specifying a Configuration File

You can override the default configuration file search paths by specifying a custom path using the `--config` flag:

```bash
go run main.go crawl --config /path/to/your/custom_config.yaml
```

## 2. Configuration

The web crawler uses [Viper](https://github.com/spf13/viper) for flexible configuration management. Settings can be defined in a `config.yaml` file, as environment variables, or through command-line flags. (Currently, only a custom `config` file can be passed via command line. Other CLI flags are under development.)

### `config.yaml` File

The primary method for configuring the crawler is through a `config.yaml` file. Viper will search for `config.yaml` (or `config.json`, `config.toml`, etc.) in the following locations, in order:

1. The current working directory (`.`).
2. `/etc/webcrawler/` (system-wide configuration).
3. `$HOME/.webcrawler/` (user-specific configuration).

A sample `config.yaml` is provided in the project root, which you can copy and modify.

```yaml
# Example config file (config.yaml)

crawler:
  useragent: "My-Custom-Crawler/1.0 (RealtimeCPI)"
  blocked_domains:
    - "*.ru"
    - "google.com"
  max_depth: 2
  concurrency: 5
  delay_seconds: 1
  ignore_robots: false
  rate_limit_backoff_seconds: 5
  max_forbidden_responses: 3
  target_urls:
    - "https://example.com/start"

http:
  timeout_seconds: 15

storage:
  provider: "gcs" # or "noop"
  gcs:
    bucket_name: "your-gcs-bucket-name"

database:
  provider: "postgres" # or "noop"
  postgres:
    dsn: "postgres://user:password@host:port/database?sslmode=disable"

queue:
  provider: "pubsub" # or "noop"
  gcp:
    project_id: "your-gcp-project-id"
    topic_id: "your-pubsub-topic-id"
```

### Environment Variables

All configuration settings can also be overridden using environment variables. The crawler expects environment variables to be prefixed with `CRAWLER_` and nested keys separated by underscores (`_`).

**Example:** To set `crawler.useragent`:

```bash
export CRAWLER_CRAWLER_USERAGENT="My-Env-Crawler/1.0"
```

To set `http.timeout_seconds`:

```bash
export CRAWLER_HTTP_TIMEOUT_SECONDS=30
```

### Configuration Precedence

Viper resolves configuration settings in the following order (from lowest to highest precedence):

1. **Defaults:** Hardcoded default values within the application.
2. **Config File:** Settings loaded from `config.yaml`.
3. **Environment Variables:** Settings specified as environment variables.
4. **Command-line flags:** Only the `--config` flag is currently supported for direct override.

This means that an environment variable will override a setting in `config.yaml`, and a setting in `config.yaml` will override a default value.

## 3. Configuration Settings Reference

This section details each available configuration option, its purpose, and examples.

### Crawler Settings

These settings control the core behavior of the web scraping engine.

* **`crawler.useragent`** (string)
  * **Description:** The User-Agent string sent with each HTTP request. It's crucial for identifying your crawler and adhering to website policies.
  * **Default:** `RealtimeCPI-Crawler/1.0 (+http://github.com/JakeFAU/realtime-cpi)`
  * **Example:**

        ```yaml
        crawler:
          useragent: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
        ```

* **`crawler.blocked_domains`** ([]string)
  * **Description:** A list of domain patterns (e.g., `google.com`, `*.ru`) the crawler must avoid. Supports leading wildcards to block entire TLDs or subtrees.
  * **Default:** `[]`
  * **Example:**

        ```yaml
        crawler:
          blocked_domains:
            - "*.ru"
            - "tracking.example.com"
        ```

* **`crawler.max_depth`** (int)
  * **Description:** The maximum number of link levels the crawler will follow from the initial target URLs. A value of `1` means it will only visit the initial URLs, `2` will visit initial URLs and links found on them, and so on.
  * **Default:** `1`
  * **Example:**

        ```yaml
        crawler:
          max_depth: 3
        ```

* **`crawler.concurrency`** (int)
  * **Description:** The maximum number of concurrent HTTP requests the crawler will make. Higher values can speed up the crawl but put more load on target servers. Be respectful and adjust this based on the target website's capacity and terms of service.
  * **Default:** `10`
  * **Example:**

        ```yaml
        crawler:
          concurrency: 20
        ```

* **`crawler.delay_seconds`** (int)
  * **Description:** The minimum delay (in seconds) between requests to the *same host*. This is a politeness setting to avoid overwhelming target servers.
  * **Default:** `2`
  * **Example:**

        ```yaml
        crawler:
          delay_seconds: 5
        ```

* **`crawler.ignore_robots`** (boolean)
  * **Description:** If `true`, the crawler will ignore `robots.txt` rules. **Use with extreme caution and only if you fully understand the implications.** It's generally recommended to respect `robots.txt`.
  * **Default:** `false`
  * **Example:**

        ```yaml
        crawler:
          ignore_robots: true
        ```

* **`crawler.rate_limit_backoff_seconds`** (int)
  * **Description:** If the crawler encounters an HTTP 429 (Too Many Requests) status code, it will pause for this many seconds before retrying requests to that domain. This helps in recovering from temporary rate limits.
  * **Default:** `5`
  * **Example:**

        ```yaml
        crawler:
          rate_limit_backoff_seconds: 10
        ```

* **`crawler.max_forbidden_responses`** (int)
  * **Description:** The maximum number of consecutive HTTP 403 (Forbidden) responses from a single host before the crawler stops visiting that host entirely. This prevents excessive retries against sites that actively block the crawler.
  * **Default:** `3`
  * **Example:**

        ```yaml
        crawler:
          max_forbidden_responses: 5
        ```

* **`crawler.target_urls`** ([]string)
  * **Description:** A list of initial URLs from which the crawler will start its operation. These are the seed URLs.
  * **Default:** `["https://www.google.com"]` (placeholder)
  * **Example:**

        ```yaml
        crawler:
          target_urls:
            - "https://example.com/products"
            - "https://anothersite.org/deals"
        ```

### HTTP Settings

Settings related to HTTP client behavior.

* **`http.timeout_seconds`** (int)
  * **Description:** The maximum time (in seconds) the crawler will wait for an HTTP response before timing out. This applies to individual requests.
  * **Default:** `15`
  * **Example:**

        ```yaml
        http:
          timeout_seconds: 30
        ```

### Storage Settings

These settings control where and how the raw HTML content of crawled pages is stored.

* **`storage.provider`** (string)
  * **Description:** Specifies the storage backend to use. Options are `"gcs"` for Google Cloud Storage or `"noop"` for a no-operation provider (which discards crawled content).
  * **Default:** (No explicit default, must be set if using a real provider)
  * **Example:**

        ```yaml
        storage:
          provider: "gcs"
        ```

* **`storage.gcs.bucket_name`** (string)
  * **Description:** Required if `storage.provider` is set to `"gcs"`. The name of the Google Cloud Storage bucket where HTML content will be saved.
  * **Default:** (No default, must be provided)
  * **Example:**

        ```yaml
        storage:
          gcs:
            bucket_name: "realtime-cpi-html-archive"
        ```

### Database Settings

These settings control where and how crawl metadata (URLs, fetch times, blob links, etc.) is stored.

* **`database.provider`** (string)
  * **Description:** Specifies the database backend for storing crawl metadata. Options are `"postgres"` for PostgreSQL or `"noop"` for a no-operation provider (which discards metadata).
  * **Default:** (No explicit default, must be set if using a real provider)
  * **Example:**

        ```yaml
        database:
          provider: "postgres"
        ```

* **`database.postgres.dsn`** (string)
  * **Description:** Required if `database.provider` is set to `"postgres"`. The PostgreSQL Data Source Name (DSN) connection string. This string contains all necessary information to connect to your PostgreSQL instance (host, port, user, password, database name, SSL mode).
  * **Default:** (No default, must be provided)
  * **Example:**

        ```yaml
        database:
          postgres:
            dsn: "postgres://user:password@localhost:5432/webcrawler_db?sslmode=disable"
        ```

### Queue Settings

These settings configure the message queue used to publish notifications about completed crawls.

* **`queue.provider`** (string)
  * **Description:** Specifies the message queue backend. Options are `"pubsub"` for Google Cloud Pub/Sub or `"noop"` for a no-operation provider (which sends no messages).
  * **Default:** (No explicit default, must be set if using a real provider)
  * **Example:**

        ```yaml
        queue:
          provider: "pubsub"
        ```

* **`queue.gcp.project_id`** (string)
  * **Description:** Required if `queue.provider` is set to `"pubsub"`. The Google Cloud Project ID where your Pub/Sub topic resides.
  * **Default:** (No default, must be provided)
  * **Example:**

        ```yaml
        queue:
          gcp:
            project_id: "your-gcp-project"
        ```

* **`queue.gcp.topic_id`** (string)
  * **Description:** Required if `queue.provider` is set to `"pubsub"`. The ID of the Google Cloud Pub/Sub topic to which crawl completion messages will be published.
  * **Default:** (No default, must be provided)
  * **Example:**

        ```yaml
        queue:
          gcp:
            topic_id: "crawl-completion-events"
        ```
