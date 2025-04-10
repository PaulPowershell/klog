package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const (
	timestampFormat = "2006-01-02T15:04:05.000"

	errorLevelJson = "error|critical|fatal"
	warnLevelJson  = "warn|warning|panic"
	debugLevelJson = "debug"

	maxConcurrency = 10 // Maximum number of concurrent log streams
)

var (
	errorRegexps = []*regexp.Regexp{
		regexp.MustCompile(`\blevel=error\b`),
		regexp.MustCompile(`\blevel=err\b`),
		regexp.MustCompile(`\blevelerror\b`),
		regexp.MustCompile(`\berr=\b`),
		regexp.MustCompile(`\[error\]`),
		regexp.MustCompile(`\[ERROR\]`),
		regexp.MustCompile(`\[err\]`),
		regexp.MustCompile(`\[ERR\]`),
		regexp.MustCompile(` ERRO: `),
		regexp.MustCompile(` Err: `),
		regexp.MustCompile(`\bERR\b`),
		regexp.MustCompile(`\bERROR\b`),
		regexp.MustCompile(`\bCRIT\b`),
		regexp.MustCompile(`\bE0\d{3}\b`), // E0***
	}

	warningRegexps = []*regexp.Regexp{
		regexp.MustCompile(`\blevel=warning\b`),
		regexp.MustCompile(`\blevel=warn\b`),
		regexp.MustCompile(`\blevelwarn\b`),
		regexp.MustCompile(`\bwarn=\b`),
		regexp.MustCompile(`\[warning\]`),
		regexp.MustCompile(`\[WARNING\]`),
		regexp.MustCompile(`\[warn\]`),
		regexp.MustCompile(`\[WARN\]`),
		regexp.MustCompile(` WARN: `),
		regexp.MustCompile(`\bWARN\b`),
		regexp.MustCompile(`\bWARNING\b`),
		regexp.MustCompile(`\bW0\d{3}\b`), // W0***
	}

	panicRegexps = []*regexp.Regexp{
		regexp.MustCompile(`\blevel=panic\b`),
		regexp.MustCompile(`\blevelpanic\b`),
		regexp.MustCompile(`\[panic\]`),
		regexp.MustCompile(`\[PANIC\]`),
		regexp.MustCompile(` panic:`),
		regexp.MustCompile(`\bPANIC\b`),
	}

	debugRegexps = []*regexp.Regexp{
		regexp.MustCompile(`\blevel=debug\b`),
		regexp.MustCompile(`\bleveldebug\b`),
		regexp.MustCompile(`\[debug\]`),
		regexp.MustCompile(`\[DEBUG\]`),
		regexp.MustCompile(` debug:`),
		regexp.MustCompile(`\bDEBUG\b`),
	}
)

var colorPalette = []pterm.Color{
	pterm.FgRed,
	pterm.FgGreen,
	pterm.FgYellow,
	pterm.FgBlue,
	pterm.FgMagenta,
	pterm.FgCyan,
	pterm.FgLightYellow,
	pterm.FgLightBlue,
	pterm.FgLightMagenta,
	pterm.FgLightCyan,
}

var (
	containerFlag     string
	keywordFlag       string
	keywordOnlyFlag   bool
	namespaceFlag     string
	timestampFlag     bool = true // Timestamp is enabled by default
	previousContainer bool
	sinceTimeFlag     int
	tailLinesFlag     int
	allPodsFlag       bool
	followFlag        bool = true // Follow logs is enabled by default
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
		// Invert switch variables if are specified
		if cmd.Flag("timestamp").Changed {
			timestampFlag = !timestampFlag
		}
		if cmd.Flag("follow").Changed {
			followFlag = !followFlag
		}
		klog(podFlag, containerFlag, keywordFlag, keywordOnlyFlag, allPodsFlag)
	},
}

func init() {
	// Set the help template for rootCmd
	rootCmd.SetHelpTemplate(rootCmd.HelpTemplate() + `
Examples:
  klog <pod-name> -c <my-container> -l	// Show logs for <my-container> in <pod-name> for last container
  klog <pod-name> -k <my-keyword>	// Show logs and color the <my-keyword> in line
  klog <pod-name> -k <my-keyword> -K 	// Show only lines and color where <my-keyword> matched
  klog <pod-name> -n <namespace>	// Show logs in the specified namespace
  klog <pod-name> -t			// Show logs without timestamp
  klog <pod-name> -p			// Show logs for the previous container in <pod-name>
  klog <pod-name> -s 24 - 50		// Show logs with sinceTime 24 hours and last 50 tailLines
  klog <pod-name> -T 50			// Show last 50 lines of logs
  klog <pod-name> -a			// Show logs from all pods that match the name
	klog <pod-name> -f			// Follow logs (default is true)
`)
	// Set flags for arguments
	rootCmd.Flags().StringVarP(&containerFlag, "container", "c", "", "Container name")
	rootCmd.Flags().StringVarP(&keywordFlag, "keyword", "k", "", "Keyword for highlighting")
	rootCmd.Flags().BoolVarP(&keywordOnlyFlag, "keywordOnly", "K", false, "Show only lines containing the keyword")
	rootCmd.Flags().StringVarP(&namespaceFlag, "namespace", "n", "", "Namespace (default is empty, meaning all namespaces)")
	rootCmd.Flags().BoolVarP(&timestampFlag, "timestamp", "t", true, "Hide timestamps in logs (default showed)")
	rootCmd.Flags().BoolVarP(&previousContainer, "previousContainer", "p", false, "Display logs for the previous container")
	rootCmd.Flags().IntVarP(&sinceTimeFlag, "sinceTime", "s", 0, "Show logs since N hours ago")
	rootCmd.Flags().IntVarP(&tailLinesFlag, "tailLines", "T", 0, "Show last N lines of logs")
	rootCmd.Flags().BoolVarP(&allPodsFlag, "allPods", "a", false, "Show logs from all pods that match the name")
	rootCmd.Flags().BoolVarP(&followFlag, "follow", "f", true, "Follow logs (default is true)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		pterm.Error.Print(err)
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

	selectedOption, err := selectorContainer.WithOptions(containerNames).Show()
	if err != nil {
		pterm.Error.Printf("Failed to select container: %v\n", err)
		return ""
	}

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
	selectedOption, err := selectorPod.WithOptions(podNames).Show() // The Show() method displays the options and waits for the user's input

	if err != nil {
		pterm.Error.Printf("Failed to select pod: %v\n", err)
		return ""
	}

	fmt.Print("\033[F\033[K\033[F\033[K") // Remove last 2 lines
	return selectedOption
}

func getPodLogOptions(containerName string) *v1.PodLogOptions {
	podLogOptions := &v1.PodLogOptions{
		Timestamps: timestampFlag,     // Show timestamps
		Follow:     followFlag,        // Follow logs
		Previous:   previousContainer, // Show logs for the previous container
		Container:  containerName,     // Container name
	}

	if sinceTimeFlag > 0 {
		sinceTime := metav1.NewTime(time.Now().Add(-time.Duration(sinceTimeFlag) * time.Hour))
		podLogOptions.SinceTime = &sinceTime
	}

	if tailLinesFlag > 0 {
		tailLines := int64(tailLinesFlag)
		podLogOptions.TailLines = &tailLines
	}
	return podLogOptions
}

func streamLogs(ctx context.Context, clientset *kubernetes.Clientset, podName, podNamespace, container string, keyword string, keywordOnly bool, showPodName bool) {
	podInfo, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		pterm.Error.Printf("Error fetching pod information for pod %s: %v\n", podName, err)
		return
	}

	selectedContainer := container
	if selectedContainer == "" {
		selectedContainer = selectContainer(podInfo.Spec.Containers)
		if selectedContainer == "" {
			return
		}
	}

	pterm.Info.Printf("Displaying logs for container '%s' in pod '%s'\n", selectedContainer, podName)

	// Construct PodLogOptions
	podLogOptions := getPodLogOptions(selectedContainer)

	// Enable log streaming
	stream, err := clientset.CoreV1().Pods(podNamespace).GetLogs(podName, podLogOptions).Stream(ctx)
	if err != nil {
		pterm.Error.Printf("Error starting log streaming for pod %s: %v\n", podName, err)
		return
	}
	defer stream.Close()

	// Select a unique color for this pod
	podColor := GetPodColor(podName)

	// Copy stream to standard output, highlighting log lines
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		// Use the unique color for this pod in the name
		PrintLogLine(podColor.Sprint(podName), scanner.Text(), keyword, keywordOnly, showPodName)
	}

	if err := scanner.Err(); err != nil {
		pterm.Error.Printf("Error reading logs for pod %s: %v\n", podName, err)
	}
}

func klog(pod string, container string, keyword string, keywordOnly bool, allPods bool) {
	// Create spinner & Start
	spinner, _ := pterm.DefaultSpinner.Start("Initialization in progress")

	var matchedPods []v1.Pod
	var namespace string = namespaceFlag // Use the specified namespace or empty

	config := LoadKubeConfig()
	ctx := context.Background()

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		pterm.Error.Printf("Error creating Kubernetes client: %v\n", err)
		os.Exit(1)
	}

	// Verify if the namespace exists if specified
	if namespace != "" && !CheckIfNamespaceExists(clientset, namespace) {
		pterm.Error.Printf("Namespace '%s' does not exist\n", namespace)
		os.Exit(1)
	}

	allPodsList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		pterm.Error.Printf("Error fetching pods: %v\n", err)
		os.Exit(1)
	}

	for _, p := range allPodsList.Items {
		if matched, _ := regexp.MatchString(pod, p.Name); matched {
			matchedPods = append(matchedPods, p)
		}
	}

	if len(matchedPods) == 0 {
		pterm.Warning.Printf("No pod found with name: %s\n", pod)
		os.Exit(1)
	}

	spinner.Success("Initialization success")

	if container == "" {
		// selection container to be done only once globally
		selectedContainer := selectContainer(matchedPods[0].Spec.Containers)
		if selectedContainer == "" {
			pterm.Error.Printf("No container selected\n")
			os.Exit(1)
		}
		container = selectedContainer
	}

	if allPods {
		var wg sync.WaitGroup
		sem := make(chan struct{}, maxConcurrency) // Limiting concurrency
		wg.Add(len(matchedPods))

		for _, p := range matchedPods {
			sem <- struct{}{}

			go func(pod v1.Pod) {
				defer func() {
					<-sem
					wg.Done()
				}()

				streamLogs(ctx, clientset, pod.Name, pod.Namespace, container, keyword, keywordOnly, true)
			}(p)
		}
		wg.Wait()
	} else {
		var podName string
		if len(matchedPods) == 0 {
			pterm.Warning.Printf("No pod found with name: %s\n", pod)
			os.Exit(1)
			return
		}

		if len(matchedPods) > 1 {
			podName = selectPod(matchedPods)
		} else {
			podName = matchedPods[0].Name
		}

		podNamespace := ""
		for _, p := range matchedPods {
			if p.Name == podName {
				podNamespace = p.Namespace
				break
			}
		}

		streamLogs(ctx, clientset, podName, podNamespace, container, keyword, keywordOnly, false)
	}
}
