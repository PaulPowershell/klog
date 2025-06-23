package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/pterm/pterm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// CheckIfNamespaceExists vérifie si un namespace existe dans Kubernetes.
func CheckIfNamespaceExists(clientset *kubernetes.Clientset, namespace string) bool {
	_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	return err == nil
}

// LoadKubeConfig charge la configuration Kubernetes à partir du fichier kubeconfig de l'utilisateur.
func LoadKubeConfig() *rest.Config {
	home, err := homedir.Dir()
	if err != nil {
		pterm.Error.Printf("Error getting home directory: %v\n", err)
		os.Exit(2)
	}
	configPath := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		pterm.Error.Printf("Error loading Kubernetes configuration: %v\n", err)
		os.Exit(2)
	}
	return config
}
