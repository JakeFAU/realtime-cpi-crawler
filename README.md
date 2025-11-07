# Realtime CPI Web Crawler

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go)
![Colly](https://img.shields.io/badge/Colly-v2-0077B6?style=for-the-badge)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)
![Google_Cloud_Storage](https://img.shields.io/badge/Google_Cloud_Storage-4285F4?style=for-the-badge&logo=google-cloud&logoColor=white)
![Google_Cloud_Pub/Sub](https://img.shields.io/badge/Google_Cloud_Pub/Sub-FFC107?style=for-the-badge&logo=google-cloud&logoColor=white)

## Project Overview

This repository contains the **high-concurrency web crawler** for the **Realtime CPI project**. Its mission is to scrape millions of product prices from the web daily, extract structured data, and feed it into our real-time price index. Designed for efficiency and scalability, this crawler is a critical component of our AI-native solution for tracking inflation.

## Key Features

* **High-Concurrency:** Built with Go, capable of managing millions of concurrent I/O operations to maximize scraping throughput.
* **Distributed & Resilient:** Designed to operate within a distributed environment, handling network failures and website changes gracefully.
* **Configurable:** Easily adaptable to different crawling targets and behaviors via a flexible configuration system.
* **Data Persistence:** Integrates with Google Cloud Storage (GCS) for raw HTML content storage and PostgreSQL for crawl metadata.
* **Event-Driven:** Publishes crawl completion events to Google Cloud Pub/Sub, enabling downstream processing by extraction and analytics services.
* **Polite Crawling:** Implements politeness policies, including `robots.txt` adherence, rate limiting, and backoff strategies to ensure responsible web scraping.

## Technologies Used

* **Go (Golang):** The primary language for its performance, concurrency model, and efficiency.
* **Colly v2:** A powerful and flexible scraping framework for Go.
* **Viper:** For robust configuration management (config files, environment variables, command-line flags).
* **Zap:** High-performance, structured logging.
* **PostgreSQL:** For storing structured crawl metadata.
* **Google Cloud Storage (GCS):** For durable and scalable storage of raw HTML content.
* **Google Cloud Pub/Sub:** For asynchronous communication and event-driven architecture.

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes. See deployment for notes on how to deploy the project on a live system.

### Prerequisites

* Go (version 1.21 or higher)
* Docker (for local PostgreSQL and Pub/Sub emulation, if desired)
* Google Cloud SDK (if interacting with real GCP services)

### Installation

1. **Clone the repository:**

    ```bash
    git clone https://github.com/JakeFAU/realtime-cpi.git
    cd realtime-cpi/webcrawler
    ```

2. **Install Go modules:**

    ```bash
    go mod tidy
    ```

### Configuration

The crawler is configured via a `config.yaml` file and/or environment variables. A sample `config.yaml` is provided in the project root. For detailed information on configuration options, refer to the [Instruction Manual](INSTRUCTIONS.md).

### Running the Crawler

To run the crawler with the default configuration:

```bash
go run main.go crawl
```

To specify a different configuration file:

```bash
go run main.go crawl --config /path/to/your/config.yaml
```

## Contributing

We welcome contributions! Please see `CONTRIBUTING.md` (if available) for details on our code of conduct, and the process for submitting pull requests to us.

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.
