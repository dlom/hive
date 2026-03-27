package deprovision

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/creds"
	"github.com/openshift/installer/pkg/types"
	"github.com/spf13/cobra"
)

// NewDeprovisionCommand is the entrypoint to create the 'deprovision' subcommand.
// It just execs "openshift-install destroy cluster".
func NewDeprovisionCommand() *cobra.Command {
	var mjSecretName string
	var dir string
	var installerBinary string
	var logLevel string

	cmd := &cobra.Command{
		Use:   "deprovision",
		Short: "Deprovision a cluster using openshift-install destroy cluster",
		Long: `Loads metadata.json from a Kubernetes Secret, configures cloud credentials,
and execs the openshift-install binary to destroy the cluster.`,
		Run: func(cmd *cobra.Command, args []string) {
			logger, err := utils.NewLogger(logLevel)
			if err != nil {
				log.Fatalf("failed to create logger: %s", err)
			}

			if mjSecretName == "" {
				logger.Fatal("--metadata-json-secret-name is required")
			}

			c, err := utils.GetClient("hiveutil-deprovision")
			if err != nil {
				logger.WithError(err).Fatal("failed to create kube client")
			}

			// Load the metadata.json Secret
			k := "METADATA_JSON_SECRET_NAME"
			os.Setenv(k, mjSecretName)
			mjSecret := utils.LoadSecretOrDie(c, k)
			if mjSecret == nil {
				logger.WithField("secretName", mjSecretName).Fatal("failed to load metadata.json Secret")
			}

			mjBytes, ok := mjSecret.Data[constants.MetadataJSONSecretKey]
			if !ok {
				logger.Fatalf("metadata.json Secret did not contain %q key", constants.MetadataJSONSecretKey)
			}

			var metadata *types.ClusterMetadata
			if err = json.Unmarshal(mjBytes, &metadata); err != nil {
				logger.WithError(err).Fatal("failed to unmarshal metadata.json")
			}

			// Write metadata.json to disk
			metadataPath := filepath.Join(dir, "metadata.json")
			if err = os.MkdirAll(dir, 0755); err != nil {
				logger.WithError(err).Fatal("failed to create output directory")
			}
			if err = os.WriteFile(metadataPath, mjBytes, 0644); err != nil {
				logger.WithError(err).Fatal("failed to write metadata.json")
			}
			logger.WithField("path", metadataPath).Info("wrote metadata.json")

			// Configure cloud credentials
			platform := metadata.Platform()
			if platform == "" {
				logger.Fatal("no platform configured in metadata.json")
			}

			configureCreds, ok := creds.ConfigureCreds[platform]
			if !ok {
				logger.WithField("platform", platform).Fatal("no credential configurator registered for platform")
			}
			configureCreds(c, metadata)
			logger.WithField("platform", platform).Info("configured credentials")

			// Exec openshift-install destroy cluster
			installerArgs := []string{
				installerBinary,
				"destroy", "cluster",
				"--dir", dir,
				"--log-level", logLevel,
			}
			logger.WithField("args", installerArgs).Info("exec-ing openshift-install")
			if err = syscall.Exec(installerBinary, installerArgs, os.Environ()); err != nil {
				logger.WithError(err).Fatal("failed to exec openshift-install")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&mjSecretName, "metadata-json-secret-name", "", "name of a Secret containing metadata.json from the installer")
	flags.StringVar(&dir, "dir", "/output", "directory to write metadata.json and use as --dir for openshift-install")
	flags.StringVar(&installerBinary, "installer-binary", "/output/openshift-install", "path to the openshift-install binary")
	flags.StringVar(&logLevel, "loglevel", "debug", "log level, one of: debug, info, warn, error, fatal, panic")

	return cmd
}
