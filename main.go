package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
)

func printLogLine(line string) {
	var logEntry map[string]interface{}

	if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
		level, exists := logEntry["level"].(string)
		if exists {
			switch level {
			case "error":
				color.Red(line)
			case "warn":
				color.Yellow(line)
			case "info":
				color.Green(line)
			default:
				fmt.Println(line)
			}
			return
		}
	}

	// Revenir à la logique d'origine pour les journaux non-JSON
	switch {
	case strings.Contains(line, "level=error"):
		color.Red(line)
	case strings.Contains(line, "levelerror"):
		color.Red(line)
	case strings.Contains(line, "level=warn"):
		color.Yellow(line)
	case strings.Contains(line, "levelwarn"):
		color.Yellow(line)
	case strings.Contains(line, "level=info"):
		color.Green(line)
	case strings.Contains(line, "levelinfo"):
		color.Green(line)
	default:
		fmt.Println(line)
	}
}

func selectContainer(containers []v1.Container) string {
	if len(containers) == 1 {
		return containers[0].Name
	}

	prompt := promptui.Select{
		Label: "Sélectionnez le conteneur:",
		Items: containers,
		Templates: &promptui.SelectTemplates{
			Label:    "{{ .Name }}",
			Active:   "\U000027A4 {{ .Name | cyan | bold }}",
			Inactive: "  {{ .Name }}",
		},
		Size:         5,
		HideSelected: true,
	}

	i, _, err := prompt.Run()
	if err != nil {
		log.Fatal("Échec de la sélection du conteneur.", err)
		os.Exit(1)
	}

	return containers[i].Name
}

func selectPod(matchedPods []v1.Pod) string {
	if len(matchedPods) == 1 {
		return matchedPods[0].Name
	}

	podNames := make([]string, len(matchedPods))
	for i, pod := range matchedPods {
		podNames[i] = pod.Name
	}

	prompt := promptui.Select{
		Label: "Sélectionnez le pod:",
		Items: podNames,
		Templates: &promptui.SelectTemplates{
			Label:    "{{ . }}",
			Active:   "\U000027A4 {{ . | cyan | bold }}",
			Inactive: "  {{ . }}",
		},
		Size:         5,
		HideSelected: true,
	}

	i, _, err := prompt.Run()
	if err != nil {
		log.Fatal("Échec de la sélection du pod.", err)
		os.Exit(1)
	}

	return podNames[i]
}

func klog(pod string, container string) {
	config, err := loadKubeConfig()
	ctx := context.Background()

	if err != nil {
		log.Fatalf("Erreur lors du chargement de la configuration Kubernetes: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Erreur lors de la création du client Kubernetes: %v\n", err)
		os.Exit(1)
	}

	allPods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Erreur lors de la récupération des pods: %v\n", err)
		os.Exit(1)
	}

	var matchedPods []v1.Pod

	for _, p := range allPods.Items {
		if matched, _ := regexp.MatchString(pod, p.Name); matched {
			matchedPods = append(matchedPods, p)
		}
	}

	if len(matchedPods) == 0 {
		log.Fatalf("Aucun pod trouvé avec le nom: %s\n", pod)
		os.Exit(1)
	}

	var selectedPodName string

	for _, p := range matchedPods {
		if p.Name == pod {
			selectedPodName = pod
			break
		}
	}

	if selectedPodName == "" {
		selectedPodName = selectPod(matchedPods)
	}

	podName := selectedPodName
	namespace := matchedPods[0].Namespace

	podInfo, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("Erreur lors de la récupération des informations du pod: %v\n", err)
		os.Exit(1)
	}

	if container == "" {
		container = selectContainer(podInfo.Spec.Containers)
	}

	fmt.Printf("Affichage du log du container '%s' dans le pod '%s'\n", container, podName)
	// Activer le suivi des journaux
	stream, err := clientset.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{
		Container: container,
		Follow:    true, // Activer le suivi des journaux par défaut
	}).Stream(ctx)
	if err != nil {
		log.Fatalf("Erreur lors du démarrage du suivi des journaux: %v\n", err)
		os.Exit(1)
	}
	defer stream.Close()

	// Copier le flux vers la sortie standard, en mettant en surbrillance les lignes de logs
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		printLogLine(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Erreur lors de la lecture des journaux: %v\n", err)
		os.Exit(1)
	}
}

func loadKubeConfig() (*rest.Config, error) {
	home := homedir.HomeDir()
	configPath := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func printHelp() {
	fmt.Println("Usage: klog [POD] [CONTAINER]")
	fmt.Println("Stream Kubernetes pod logs.")
	fmt.Println("Options:")
	fmt.Println("  -h, --help       Show this help message and exit.")
	fmt.Println("Examples:")
	fmt.Println("  klog my-pod - Select containers and show logs for 'my-pod'.")
	fmt.Println("  klog my-pod my-container - Show logs for 'my-container' in 'my-pod'.")
}

func main() {
	helpFlag := flag.Bool("h", false, "Show help message")

	flag.Parse()

	if *helpFlag {
		printHelp()
		os.Exit(0)
	}

	pod := flag.Arg(0)
	container := flag.Arg(1)

	if pod == "" {
		log.Fatalf("Le nom du pod est obligatoire.")
		os.Exit(1)
	}

	klog(pod, container)
}
