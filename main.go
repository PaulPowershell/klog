package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
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

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

const (
	timestampFormat = "2006-01-02 15:04:05.999999999 MST"
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
	// Définir le modèle d'aide pour rootCmd
	rootCmd.SetHelpTemplate(rootCmd.HelpTemplate() + `
Exemples:
  klog -p my-pod -t / Select containers and show logs for 'my-pod' with timestamp
  klog -p my-pod -c my-container -l / Show logs for 'my-container' in 'my-pod' for last container
  klog -p my-pod -c my-container -k 'my-keyword' / Show logs for 'my-container' in 'my-pod' and color the 'my-keyword' in line
`)
	// Définir les flags pour les arguments
	rootCmd.Flags().StringVarP(&podFlag, "pod", "p", "", "Nom du pod (obligatoire)")
	rootCmd.Flags().StringVarP(&containerFlag, "container", "c", "", "Nom du conteneur")
	rootCmd.Flags().StringVarP(&keywordFlag, "keyword", "k", "", "Mot clé pour la mise en surbrillance")
	rootCmd.Flags().BoolVarP(&timestampFlag, "timestamp", "t", false, "Afficher les horodatages dans les logs")
	rootCmd.Flags().BoolVarP(&lastContainer, "lastContainer", "l", false, "Afficher les logs du container précédent")
}

// Fonction pour mettre en surbrillance un mot dans la chaîne
func highlightKeyword(line string, keyword string, colorFunc func(a ...interface{}) string) string {
	magenta := color.New(color.FgMagenta).SprintFunc()
	re := regexp.MustCompile(keyword)
	matches := re.FindAllStringIndex(line, -1)

	if len(matches) > 0 {
		result := ""
		startIndex := 0
		for _, match := range matches {
			result += colorFunc(line[startIndex:match[0]]) + magenta(line[match[0]:match[1]])
			startIndex = match[1]
		}
		result += colorFunc(line[startIndex:])
		return result
	}

	return colorFunc(line)
}

func cleanColorCodes(line string) string {
	// Définir l'expression régulière pour les séquences de contrôle de couleur ANSI
	colorRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)

	// Supprimer les séquences de contrôle de couleur ANSI de la ligne
	cleanLine := colorRegex.ReplaceAllString(line, "")

	return cleanLine
}

func printLogLine(line string, keyword string) {
	var logEntry map[string]interface{}
	var colorFunc func(a ...interface{}) string
	var timestamp string

	if timestampFlag {
		// Extraire l'horodatage et le reste de la ligne
		if parts := strings.SplitN(line, " ", 2); len(parts) == 2 {
			timestamp = parts[0]
			line = parts[1]
		}
	}

	lowerLine := strings.ToLower(line)

	switch {
	// case strings.Contains(lowerLine, "level=error"), strings.Contains(lowerLine, "levelerror"):
	case strings.Contains(lowerLine, "error"):
		colorFunc = color.New(color.FgRed).SprintFunc()
	case strings.Contains(lowerLine, "warn"):
		colorFunc = color.New(color.FgYellow).SprintFunc()
	case strings.Contains(lowerLine, "panic"):
		colorFunc = color.New(color.FgYellow).SprintFunc()
	case strings.Contains(lowerLine, "debug"):
		colorFunc = color.New(color.FgCyan).SprintFunc()
	default:
		colorFunc = color.New(color.FgWhite).SprintFunc()
	}

	if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
		level, exists := logEntry["level"].(string)
		if exists {
			switch strings.ToLower(level) {
			case "error":
				colorFunc = color.New(color.FgRed).SprintFunc()
			case "warn":
				colorFunc = color.New(color.FgYellow).SprintFunc()
			case "panic":
				colorFunc = color.New(color.FgYellow).SprintFunc()
			case "debug":
				colorFunc = color.New(color.FgCyan).SprintFunc()
			default:
				colorFunc = color.New(color.FgWhite).SprintFunc()
			}
		}
	}

	// Convertir la chaîne d'horodatage en objet time.Time
	if timestamp != "" {
		t, err := time.Parse(time.RFC3339Nano, timestamp)
		if err == nil {
			timestamp = t.Format(timestampFormat)
		}
	}

	// Appliquer la colorisation au reste de la ligne
	coloredLine := highlightKeyword(colorFunc(line), keyword, colorFunc)

	// Supprimer les séquences de contrôle de couleur ANSI de la ligne
	cleanLine := cleanColorCodes(coloredLine)

	// Afficher l'horodatage normalement et le reste coloré
	fmt.Printf("%s %s\n", timestamp, cleanLine)
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

func klog(pod string, container string, keyword string) {
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
		Container:  container,
		Timestamps: timestampFlag, // Afficher les horodatages
		Follow:     true,          // Activer le suivi des journaux par défaut
		Previous:   lastContainer, // Afficher les journaux du précédent container
	}).Stream(ctx)
	if err != nil {
		log.Fatalf("Erreur lors du démarrage du suivi des journaux: %v\n", err)
		os.Exit(1)
	}
	defer stream.Close()

	// Copier le flux vers la sortie standard, en mettant en surbrillance les lignes de logs
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		// Utiliser la fonction pour mettre en surbrillance le mot clé
		printLogLine(scanner.Text(), keyword)
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

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
