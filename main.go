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
	// Définir le modèle d'aide pour rootCmd
	rootCmd.SetHelpTemplate(rootCmd.HelpTemplate() + `
Exemples:
  klog -p my-pod -t / Select containers and show logs for 'my-pod' with timestamp
  klog -p my-pod -c my-container -l / Show logs for 'my-container' in 'my-pod' for last container
  klog -p my-pod -c my-container -k 'my-keyword' / Show logs for 'my-container' in 'my-pod' and color the 'my-keyword' in line
`)
	// Définir les flags pour les arguments
	rootCmd.Flags().StringVarP(&podFlag, "pod", "p", "", "Nom du pod (obligatoire)")
	rootCmd.MarkFlagRequired("pod")
	rootCmd.Flags().StringVarP(&containerFlag, "container", "c", "", "Nom du conteneur")
	rootCmd.Flags().StringVarP(&keywordFlag, "keyword", "k", "", "Mot clé pour la mise en surbrillance")
	rootCmd.Flags().BoolVarP(&timestampFlag, "timestamp", "t", false, "Afficher les horodatages dans les logs")
	rootCmd.Flags().BoolVarP(&lastContainer, "lastContainer", "l", false, "Afficher les logs du container précédent")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

// Fonction pour mettre en surbrillance un mot dans la chaîne
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
		// Extraire l'horodatage et le reste de la ligne
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
			case "panic":
				colorFunc = pterm.Yellow
			case "debug":
				colorFunc = pterm.Cyan
			default:
				colorFunc = pterm.White
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

	if keyword == "" {
		fmt.Printf("%s %s\n", pterm.FgDarkGray.Sprint(timestamp), colorFunc(line))
	} else {
		// Appliquer la colorisation au reste de la ligne
		coloredLine := highlightKeyword(colorFunc(line), keyword, colorFunc)

		// Afficher l'horodatage normalement et le reste coloré
		fmt.Printf("%s %s\n", pterm.FgDarkGray.Sprint(timestamp), coloredLine)
	}
}

func selectContainer(containers []v1.Container) string {
	// Si un seul conteneur est disponible, retourner son nom directement
	if len(containers) == 1 {
		return containers[0].Name
	}

	// Utiliser les noms des conteneurs dans l'interface interactive
	selectorContainer := pterm.DefaultInteractiveSelect.WithDefaultText("Select a container")
	selectorContainer.MaxHeight = 10

	// Créer une tranche de chaînes pour stocker les noms des conteneurs
	containerNames := make([]string, len(containers))
	for i, container := range containers {
		containerNames[i] = container.Name
	}

	selectedOption, _ := selectorContainer.WithOptions(containerNames).Show()

	fmt.Print("\033[F\033[K\033[F\033[K") // Supprimer les 2 dernieres lignes
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

	fmt.Print("\033[F\033[K\033[F\033[K") // Supprimer les 2 dernieres lignes
	return selectedOption
}

func klog(pod string, container string, keyword string) {
	// Create spinner & Start
	spinner, _ := pterm.DefaultSpinner.Start("Initialisation en cours")

	config, err := loadKubeConfig()
	ctx := context.Background()

	if err != nil {
		pterm.Error.Printf("Erreur lors du chargement de la configuration Kubernetes: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		pterm.Error.Printf("Erreur lors de la création du client Kubernetes: %v\n", err)
		os.Exit(1)
	}

	allPods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		pterm.Error.Printf("Erreur lors de la récupération des pods: %v\n", err)
		os.Exit(1)
	}

	var matchedPods []v1.Pod

	for _, p := range allPods.Items {
		if matched, _ := regexp.MatchString(pod, p.Name); matched {
			matchedPods = append(matchedPods, p)
		}
	}

	if len(matchedPods) == 0 {
		pterm.Error.Printf("Aucun pod trouvé avec le nom: %s\n", pod)
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

	if selectedPodName == "" {
		selectedPodName = selectPod(matchedPods)
	}

	podName := selectedPodName
	namespace := matchedPods[0].Namespace

	podInfo, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		pterm.Error.Printf("Erreur lors de la récupération des informations du pod: %v\n", err)
		os.Exit(1)
	}

	if container == "" {
		container = selectContainer(podInfo.Spec.Containers)
	}

	pterm.Info.Printf("Affichage du log du container '%s' dans le pod '%s'\n", container, podName)

	// Activer le suivi des journaux
	stream, err := clientset.CoreV1().Pods(namespace).GetLogs(podName, &v1.PodLogOptions{
		Container:  container,
		Timestamps: timestampFlag, // Afficher les horodatages
		Follow:     true,          // Activer le suivi des journaux par défaut
		Previous:   lastContainer, // Afficher les journaux du précédent container
	}).Stream(ctx)
	if err != nil {
		pterm.Error.Printf("Erreur lors du démarrage du suivi des journaux: %v\n", err)
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
		pterm.Error.Printf("Erreur lors de la lecture des journaux: %v\n", err)
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
