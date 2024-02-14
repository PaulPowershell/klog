# Klog - Kubernetes Pod Logs Viewer

Klog is a command-line tool for streaming logs from Kubernetes pods. It allows you to easily view and follow logs from specific pods and containers.

## Prerequisites

Before using this application, ensure you have the following prerequisites:

- Go installed on your system. (to build)
- `kubectl` configured with access to your Kubernetes cluster.

## Installation

1. Clone the repository to your local machine:

```bash
git clone https://github.com/yourusername/klog
cd klog
```

Build the Go application:
go build .

Usage
To view logs for a specific pod, run the application with the pod name as an argument:
./klog <pod-name>

To view logs for a specific pod and container, provide both the pod and container names as arguments:
./klog <pod-name> <container-name>

Options
-h, --help: Show help message.
Output
The application streams the logs in real-time. It uses color-coding to highlight different log levels:

Errors: Red
Warnings: Yellow
Info: Green
Other: Default color
Demo

License
This project is licensed under the MIT License. See the LICENSE file for details.

Replace "yourusername" with your actual GitHub username, and update the image URLs accordingly. Customize the content as needed. If you have any additional requests or changes, feel free to let me know!
