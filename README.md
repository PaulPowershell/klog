# Klog - Kubernetes Pod Logs Viewer
Klog is a command-line tool for streaming logs from Kubernetes pods. It allows you to easily view and follow logs from specific pods and containers.

## Last Build
[![Go Build & Release](https://github.com/VegaCorporoptions/klog/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/VegaCorporoptions/klog/actions/workflows/go.yml)

## Prerequisites

Before using this application, ensure you have the following prerequisites:

- Go installed on your system. (to build)
- `kubectl` configured with access to your Kubernetes cluster.

## Installation
Clone the repository to your local machine:

```bash
git clone https://github.com/yourusername/klog
cd your-repo
```

Build the Go application:
```bash
go build .
```

## Download Ksub Executable
You can download the executable for Ksub directly from the latest release with its version. This allows you to use Ksub without the need to build it yourself. Here are the steps to download the executable for your system:
Visit the [Releases](https://github.com/VegaCorporoptions/Klog/releases/latest) page.

Usage
To view logs for a specific pod, run the application with the pod name as an argument:
Run the Ksub application:
```bash
klog <[mandatory]pod name> <[option]container name>
klog -h
```
Select `pod` or `container` if you have multiple choices

## Demo
![klog.gif](https://github.com/VegaCorporoptions/Klog/blob/main/klog.gif?raw=true)

License
This project is licensed under the MIT License. See the LICENSE file for details.
