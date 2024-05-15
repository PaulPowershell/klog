package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const (
	timestampFormat = "2006-01-02T15:04:05.000"
)

var (
	podFlag       string
	containerFlag string
	keywordFlag   string
	timestampFlag bool
	lastContainer bool
)

var rootCmd = &cobra.Command{
	Use:   "klog",
	Short: "Stream Kubernetes pod logs.",
	Run: func(cmd *cobra.Command, args []string) {
		klog(podFlag, containerFlag, keywordFlag)
	},
}

func init() {
	// Set the help template for rootCmd
	rootCmd.SetHelpTemplate(rootCmd.HelpTemplate() + `
Examples:
  klog -p my-pod -t / Select containers and show logs for 'my-pod' with timestamp
  klog -p my-pod -c my-container -l / Show logs for 'my-container' in 'my-pod' for last container
  klog -p my-pod -c my-container -k 'my-keyword' / Show logs for 'my-container' in 'my-pod' and color the 'my-keyword' in line
`)
	// Set flags for arguments
	rootCmd.Flags().StringVarP(&podFlag, "pod", "p", "", "Pod name (required)")
	rootCmd.MarkFlagRequired("pod")
	rootCmd.Flags().StringVarP(&containerFlag, "container", "c", "", "Container name")
	rootCmd.Flags().StringVarP(&keywordFlag, "keyword", "k", "", "Keyword for highlighting")
	rootCmd.Flags().BoolVarP(&timestampFlag, "timestamp", "t", false, "Display timestamps in logs")
	rootCmd.Flags().BoolVarP(&lastContainer, "lastContainer", "l", false, "Display logs for the previous container")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		pterm.Error.Print(err)
	}
}

// Function to highlight a word in the string
func highlightKeyword(line string, keyword string, colorFunc func(a ...interface{}) string) string {
	re := regexp.MustCompile(keyword)
	matches := re.FindAllStringIndex(line, -1)

	if len(matches) > 0 {
		result := ""
		startIndex := 0
		for _, match := range matches {
			result += colorFunc(line[startIndex:match[0]]) + pterm.BgMagenta.Sprint(line[match[0]:match[1]])
			startIndex = match[1]
		}
		result += colorFunc(line[startIndex:])
		return result
	}

	return colorFunc(line)
}

func printLogLine(line string, keyword string) {
	var logEntry map[string]interface{}
	var colorFunc func(a ...interface{}) string
	var timestamp string

	if timestampFlag {
		// Extract timestamp and rest of the line
		if parts := strings.SplitN(line, " ", 2); len(parts) == 2 {
			timestamp = parts[0]
			line = parts[1]
		}
	}

	switch {
	case strings.Contains(line, "level=error"), strings.Contains(line, "levelerror"), strings.Contains(line, "ERROR"):
		colorFunc = pterm.Red
	case strings.Contains(line, "level=warn"), strings.Contains(line, "levelwarn"), strings.Contains(line, "WARN"):
		colorFunc = pterm.Yellow
	case strings.Contains(line, "level=warning"), strings.Contains(line, "levelwarn"), strings.Contains(line, "WARN"):
		colorFunc = pterm.Yellow
	case strings.Contains(line, "level=panic"), strings.Contains(line, "levelpanic"), strings.Contains(line, "PANIC"):
		colorFunc = pterm.Yellow
	case strings.Contains(line, "level=debug"), strings.Contains(line, "leveldebug"), strings.Contains(line, "DEBUG"):
		colorFunc = pterm.Cyan
	default:
		colorFunc = pterm.White
	}

	if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
		level, exists := logEntry["level"].(string)
		if exists {
			switch strings.ToLower(level) {
			case "error":
				colorFunc = pterm.Red
			case "warn":
				colorFunc = pterm.Yellow
			case "warning":
				colorFunc = pterm.Yellow
			case "panic":
				colorFunc = pterm.Yellow
			case "debug":
				colorFunc = pterm.Cyan
			default:
				colorFunc = pterm.White
			}
		}
	}

	// Convert timestamp string to time.Time object
	if timestamp != "" {
		t, err := time.Parse(time.RFC3339Nano, timestamp)
		if err == nil {
			timestamp = t.Format(timestampFormat)
		}
	}

	if keyword == "" {
		fmt.Printf("%s %s\n", pterm.FgDarkGray.Sprint(timestamp), colorFunc(line))
	} else {
		// Apply colorization to the rest of the line
		coloredLine := highlightKeyword(colorFunc(line), keyword, colorFunc)

		// Print timestamp normally and the rest colored
		fmt.Printf("%s %s\n", pterm.FgDarkGray.Sprint(timestamp), coloredLine)
	}
}

func selectContainer(containers []v1.Container) string {
	// If only one container is available, return its name directly
	if len(containers) == 1 {
		return containers[0].Name
	}

	// Use container names in interactive interface
	selectorContainer := pterm.DefaultInteractiveSelect.WithDefaultText("Select a container")
	selectorContainer.MaxHeight = 10

	// Create a slice of strings to store container names
	containerNames := make([]string, len(containers))
	for i, container := range containers {
		containerNames[i] = container.Name
	}

	selectedOption, _ := selectorContainer.WithOptions(containerNames).Show()

	fmt.Print("\033[F\033[K\033[F\033[K") // Remove last 2 lines
	return selectedOption
}

func selectPod(matchedPods []v1.Pod) string {
	if len(matchedPods) == 1 {
		return matchedPods[0].Name
	}

	podNames := make([]string, len(matchedPods))
	for i, pod := range matchedPods {
		podNames[i] = pod.Name
	}

	selectorPod := pterm.DefaultInteractiveSelect.WithDefaultText("Select a pod")
	selectorPod.MaxHeight = 10
	selectedOption, _ := selectorPod.WithOptions(podNames).Show() // The Show() method displays the options and waits for the user's input

	fmt.Print("\033[F\033[K\033[F\033[K") // Remove last 2 lines
	return selectedOption
}

func klog(pod string, container string, keyword string) {
	// Create spinner & Start
	spinner, _ := pterm.DefaultSpinner.Start("Initialization in progress")

	var matchedPods []v1.Pod
	var namespace string
	var selectedPodName string
	var podName string

	config := loadKubeConfig()
	ctx := context.Background()

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		pterm.Error.Printf("Error creating Kubernetes client: %v\n", err)
		os.Exit(1)
	}

	allPods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		pterm.Error.Printf("Error fetching pods: %v\n", err)
		os.Exit(1)
	}

	for _, p := range allPods.Items {
		if matched, _ := regexp.MatchString(pod, p.Name); matched {
			matchedPods = append(matchedPods, p)
		}
	}

	if len(matchedPods) == 0 {
		pterm.Error.Printf("No pod found with name: %s\n", pod)
		os.Exit(1)
	}

	for _, p := range matchedPods {
		if p.Name == pod {
			selectedPodName = pod
			break
		}
	}

	spinner.Success("Initialization success")

	if selectedPodName == "" {
		podName = selectPod(matchedPods)
	}

	for _, p := range matchedPods {
		if p.Name == podName {
			namespace = p.Namespace
			break
		}
	}

	podInfo, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		pterm.Error.Printf("Error fetching pod information: %v\n", err)
		os.Exit(1)
	}

	if container == "" {
		container = selectContainer(podInfo.Spec.Containers)
	}

	pterm.Info.Printf("Displaying logs for container '%s' in pod '%s'\n", container, podName)

	// Enable log streaming
	stream, err := clientset.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{
		Container:  container,
		Timestamps: timestampFlag, // Display timestamps
		Follow:     true,          // Enable log streaming by default
		Previous:   lastContainer, // Display logs of the previous container
	}).Stream(ctx)
	if err != nil {
		pterm.Error.Printf("Error starting log streaming: %v\n", err)
		os.Exit(1)
	}
	defer stream.Close()

	// Copy stream to standard output, highlighting log lines
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		// Use function to highlight keyword
		printLogLine(scanner.Text(), keyword)
	}

	if err := scanner.Err(); err != nil {
		pterm.Error.Printf("Error reading logs: %v\n", err)
		os.Exit(1)
	}
}

func loadKubeConfig() *rest.Config {
	home := homedir.HomeDir()
	configPath := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		pterm.Error.Printf("Error loading Kubernetes configuration: %v\n", err)
		os.Exit(1)
	}
	return config
}
