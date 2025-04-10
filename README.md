# Klog - Kubernetes Pod Logs Viewer
Klog is a command-line tool for streaming logs from Kubernetes pods. It allows you to easily view and follow logs from specific pods and containers.

## Last Build
[![Go Build & Release](https://github.com/VegaCorporoptions/klog/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/VegaCorporoptions/klog/actions/workflows/go.yml)

## Prerequisites

Before using this application, ensure you have the following prerequisites:

- Go installed on your system (to build)
- `kubectl` configured with access to your Kubernetes cluster

## Installation (optionnal)
Clone the repository to your local machine:

```bash
git clone https://github.com/yourusername/klog
cd your-repo
```

Build the Go application:
```bash
go build .
```

## Download Klog Executable
You can download the executable for Klog directly from the latest release with its version. This allows you to use Klog without the need to build it yourself. Here are the steps to download the executable for your system:
Visit the [Releases](https://github.com/VegaCorporoptions/Klog/releases/latest) page.

## Usage
To view logs for a specific pod, run the application with the pod name as an argument:
Run the Klog application:
```yaml
Usage:
  klog [flags]

Flags:
  -a, --allPods             Show logs from all pods that match the name
  -c, --container string    Container name
  -f, --follow              Follow logs (default is true) (default true)
  -h, --help                help for klog
  -k, --keyword string      Keyword for highlighting
  -K, --keywordOnly         Show only lines containing the keyword
  -n, --namespace string    Namespace (default is empty, meaning all namespaces)
  -p, --previousContainer   Display logs for the previous container
  -s, --sinceTime int       Show logs since N hours ago
  -T, --tailLines int       Show last N lines of logs
  -t, --timestamp           Hide timestamps in logs (default showed) (default true)

Examples:
  klog <pod-name> -a                    // Show logs from all pods that match the name
  klog <pod-name> -c <my-container> -l  // Show logs for <my-container> in <pod-name> for last container
  klog <pod-name> -k <my-keyword>       // Show logs and color the <my-keyword> in line
  klog <pod-name> -f                    // Follow logs (default is true)
  klog <pod-name> -k <my-keyword> -K    // Show only lines and color where <my-keyword> matched
  klog <pod-name> -n <namespace>        // Show logs in the specified namespace
  klog <pod-name> -p                    // Show logs for the previous container in <pod-name>
  klog <pod-name> -s 24 - 50            // Show logs with sinceTime 24 hours and last 50 tailLines
  klog <pod-name> -T 50                 // Show last 50 lines of logs
  klog <pod-name> -t                    // Show logs without timestamp
```
You can select `pod` or `container` if you have multiple choices

## Demo
![klog.gif](klog.gif)

License
This project is licensed under the MIT License. See the LICENSE file for details.
