/*
Copyright 2021 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/fluxcd/flux2/v2/internal/utils"
	"github.com/fluxcd/flux2/v2/pkg/manifestgen/sourcesecret"
)

var createSecretHelmCmd = &cobra.Command{
	Use:   "helm [name]",
	Short: "Create or update a Kubernetes secret for Helm repository authentication",
	Long:  `The create secret helm command generates a Kubernetes secret with basic authentication credentials.`,
	Example: ` # Create a Helm authentication secret on disk and encrypt it with Mozilla SOPS
  flux create secret helm repo-auth \
    --namespace=my-namespace \
    --username=my-username \
    --password=my-password \
    --export > repo-auth.yaml

  sops --encrypt --encrypted-regex '^(data|stringData)$' \
    --in-place repo-auth.yaml`,

	RunE: createSecretHelmCmdRun,
}

type secretHelmFlags struct {
	username string
	password string
	secretTLSFlags
}

var secretHelmArgs secretHelmFlags

func init() {
	flags := createSecretHelmCmd.Flags()
	flags.StringVarP(&secretHelmArgs.username, "username", "u", "", "basic authentication username")
	flags.StringVarP(&secretHelmArgs.password, "password", "p", "", "basic authentication password")
	flags.StringVar(&secretHelmArgs.tlsCrtFile, "tls-crt-file", "", "TLS authentication cert file path")
	flags.StringVar(&secretHelmArgs.tlsKeyFile, "tls-key-file", "", "TLS authentication key file path")
	flags.StringVar(&secretHelmArgs.caCrtFile, "ca-crt-file", "", "TLS authentication CA file path")

	createSecretCmd.AddCommand(createSecretHelmCmd)
}

func createSecretHelmCmdRun(cmd *cobra.Command, args []string) error {
	name := args[0]

	labels, err := parseLabels()
	if err != nil {
		return err
	}

	caBundle := []byte{}
	if secretHelmArgs.caCrtFile != "" {
		var err error
		caBundle, err = os.ReadFile(secretHelmArgs.caCrtFile)
		if err != nil {
			return fmt.Errorf("unable to read TLS CA file: %w", err)
		}
	}

	var certFile, keyFile []byte
	if secretHelmArgs.tlsCrtFile != "" {
		if certFile, err = os.ReadFile(secretHelmArgs.tlsCrtFile); err != nil {
			return fmt.Errorf("failed to read cert file: %w", err)
		}
	}
	if secretHelmArgs.tlsKeyFile != "" {
		if keyFile, err = os.ReadFile(secretHelmArgs.tlsKeyFile); err != nil {
			return fmt.Errorf("failed to read key file: %w", err)
		}
	}

	opts := sourcesecret.Options{
		Name:      name,
		Namespace: *kubeconfigArgs.Namespace,
		Labels:    labels,
		Username:  secretHelmArgs.username,
		Password:  secretHelmArgs.password,
		CACrt:     caBundle,
		TLSCrt:    certFile,
		TLSKey:    keyFile,
	}
	secret, err := sourcesecret.GenerateHelm(opts)
	if err != nil {
		return err
	}

	if createArgs.export {
		rootCmd.Println(secret.Content)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()
	kubeClient, err := utils.KubeClient(kubeconfigArgs, kubeclientOptions)
	if err != nil {
		return err
	}
	var s corev1.Secret
	if err := yaml.Unmarshal([]byte(secret.Content), &s); err != nil {
		return err
	}
	if err := upsertSecret(ctx, kubeClient, s); err != nil {
		return err
	}

	logger.Actionf("helm secret '%s' created in '%s' namespace", name, *kubeconfigArgs.Namespace)
	return nil
}
