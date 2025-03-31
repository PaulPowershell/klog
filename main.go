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
	errorKeywords   = "level=error|level=err|levelerror|err=|[error]|[ERROR]|[err]|[ERR]| ERRO: | Err: | ERR | ERROR | CRIT "
	warningKeywords = "level=warning|level=warn|levelwarn|warn=|[warning]|[WARNING]|[warn]|[WARN]| WARN: | WARN | WARNING |W0331 "
	panicKeywords   = "level=panic|levelpanic|[panic]|[PANIC]| panic:|PANIC "
	debugKeywords   = "level=debug|leveldebug|[debug]|[DEBUG]| debug:|DEBUG "

	errorLevelJson = "error|critical|fatal"
	warnLevelJson  = "warn|warning|panic"
	debugLevelJson = "debug"
)

var (
	containerFlag string
	keywordFlag   string
	namespaceFlag string
	timestampFlag bool = true // Timestamp is enabled by default
	lastContainer bool
	sinceTimeFlag int
	tailLinesFlag int
)

var rootCmd = &cobra.Command{
	Use:   "klog",
	Short: "Stream Kubernetes pod logs.",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			pterm.Error.Println("Pod name required")
			_ = cmd.Usage()
			os.Exit(128)
		}

		podFlag := args[0]
		// Invert the timestampFlag if -t is specified
		if cmd.Flag("timestamp").Changed {
			timestampFlag = !timestampFlag
		}
		klog(podFlag, containerFlag, keywordFlag)
	},
}

func init() {
	// Set the help template for rootCmd
	rootCmd.SetHelpTemplate(rootCmd.HelpTemplate() + `
Examples:
  klog <pod-name> -t			// Select containers and show logs for <pod-name> without timestamp
  klog <pod-name> -c <my-container> -l	// Show logs for <my-container> in <pod-name> for last container
  klog <pod-name> -k <my-keyword>	// Show logs for <pod-name> and color the <my-keyword> in line
  klog <pod-name> -s 24 - 50		// Show logs for <pod-name> with sinceTime 24 hours and last 50 tailLines
  klog <pod-name> -n <namespace>	// Show logs for <pod-name> in the specified namespace
`)
	// Set flags for arguments
	rootCmd.Flags().StringVarP(&containerFlag, "container", "c", "", "Container name")
	rootCmd.Flags().StringVarP(&keywordFlag, "keyword", "k", "", "Keyword for highlighting")
	rootCmd.Flags().StringVarP(&namespaceFlag, "namespace", "n", "", "Namespace (default is empty, meaning all namespaces)")
	rootCmd.Flags().BoolVarP(&timestampFlag, "timestamp", "t", true, "Hide timestamps in logs (default showed)")
	rootCmd.Flags().BoolVarP(&lastContainer, "lastContainer", "l", false, "Display logs for the previous container")
	rootCmd.Flags().IntVarP(&sinceTimeFlag, "sinceTime", "s", 0, "Show logs since N hours ago")
	rootCmd.Flags().IntVarP(&tailLinesFlag, "tailLines", "T", 0, "Show last N lines of logs")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		pterm.Error.Print(err)
	}
}

func checkIfNamespaceExists(clientset *kubernetes.Clientset, namespace string) bool {
	_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	return err == nil
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

func containsAny(line string, substrings ...string) bool {
	for _, s := range substrings {
		if strings.Contains((line), s) {
			return true
		}
	}
	return false
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
	case containsAny(line, strings.Split(errorKeywords, "|")...):
		colorFunc = pterm.Red
	case containsAny(line, strings.Split(warningKeywords, "|")...):
		colorFunc = pterm.Yellow
	case containsAny(line, strings.Split(panicKeywords, "|")...):
		colorFunc = pterm.Yellow
	case containsAny(line, strings.Split(debugKeywords, "|")...):
		colorFunc = pterm.Cyan
	default:
		colorFunc = pterm.White
	}

	if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
		level, exists := logEntry["level"].(string)
		if exists {
			levelLower := strings.ToLower(level)
			switch {
			case containsAny(levelLower, strings.Split(errorLevelJson, "|")...):
				colorFunc = pterm.Red
			case containsAny(levelLower, strings.Split(warnLevelJson, "|")...):
				colorFunc = pterm.Yellow
			case containsAny(levelLower, strings.Split(debugLevelJson, "|")...):
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
	var namespace string = namespaceFlag // Utilisation du namespace spécifié ou vide

	config := loadKubeConfig()
	ctx := context.Background()

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		pterm.Error.Printf("Error creating Kubernetes client: %v\n", err)
		os.Exit(1)
	}

	// Vérifie si le namespace existe si spécifié
	if namespace != "" && !checkIfNamespaceExists(clientset, namespace) {
		pterm.Error.Printf("Namespace '%s' does not exist\n", namespace)
		os.Exit(1)
	}

	allPods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
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
		pterm.Warning.Printf("No pod found with name: %s\n", pod)
		os.Exit(1)
	}

	var selectedPodName string
	for _, p := range matchedPods {
		if p.Name == pod {
			selectedPodName = pod
			break
		}
	}

	spinner.Success("Initialization success")

	var podName string
	if selectedPodName == "" {
		podName = selectPod(matchedPods)
	} else {
		podName = selectedPodName
	}

	var podNamespace string
	for _, p := range matchedPods {
		if p.Name == podName {
			podNamespace = p.Namespace
			break
		}
	}

	podInfo, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		pterm.Error.Printf("Error fetching pod information: %v\n", err)
		os.Exit(1)
	}

	if container == "" {
		container = selectContainer(podInfo.Spec.Containers)
	}

	pterm.Info.Printf("Displaying logs for container '%s' in pod '%s'\n", container, podName)

	// Construct PodLogOptions
	podLogOptions := &v1.PodLogOptions{
		Container:  container,
		Timestamps: timestampFlag, // Display timestamps
		Follow:     true,          // Enable log streaming by default
		Previous:   lastContainer, // Display logs of the previous container
	}

	if sinceTimeFlag > 0 {
		sinceTime := metav1.NewTime(time.Now().Add(-time.Duration(sinceTimeFlag) * time.Hour))
		podLogOptions.SinceTime = &sinceTime
	}

	if tailLinesFlag > 0 {
		tailLines := int64(tailLinesFlag)
		podLogOptions.TailLines = &tailLines
	}

	// Enable log streaming
	stream, err := clientset.CoreV1().Pods(podNamespace).GetLogs(podName, podLogOptions).Stream(ctx)
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
		os.Exit(2)
	}
	return config
}
