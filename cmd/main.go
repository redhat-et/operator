/*
Copyright 2023.

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
	"crypto/tls"
	"flag"

	"github.com/securesign/operator/internal/images"

	"net/http"
	"os"
	"strconv"

	"k8s.io/klog/v2"

	"k8s.io/utils/ptr"

	"github.com/securesign/operator/internal/metrics"
	"sigs.k8s.io/controller-runtime/pkg/config"

	consolev1 "github.com/openshift/api/console/v1"
	v1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/securesign/operator/internal/clidownload"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/ctlog"
	"github.com/securesign/operator/internal/controller/fulcio"
	"github.com/securesign/operator/internal/controller/rekor"
	"github.com/securesign/operator/internal/controller/securesign"
	"github.com/securesign/operator/internal/controller/trillian"
	"github.com/securesign/operator/internal/controller/tsa"
	"github.com/securesign/operator/internal/controller/tuf"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

//+kubebuilder:rbac:groups=console.openshift.io,resources=consoleclidownloads,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(rhtasv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(consolev1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		pprofAddr            string
		secureMetrics        bool
		enableHTTP2          bool
	)

	flag.StringVar(&pprofAddr, "pprof-address", "", "The address to expose the pprof server. Default is empty string which disables the pprof server.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.Int64Var(&constants.CreateTreeDeadline, "create-tree-deadline", constants.CreateTreeDeadline, "The time allowance (in seconds) for the create tree job to run before failing.")
	utils.BoolFlagOrEnv(&constants.Openshift, "openshift", "OPENSHIFT", false, "Enable to ensures the operator applies OpenShift specific configurations.")
	utils.RelatedImageFlag("trillian-log-signer-image", images.TrillianLogSigner, "The image used for trillian log signer.")
	utils.RelatedImageFlag("trillian-log-server-image", images.TrillianServer, "The image used for trillian log server.")
	utils.RelatedImageFlag("trillian-db-image", images.TrillianDb, "The image used for trillian's database.")
	utils.RelatedImageFlag("trillian-netcat-image", images.TrillianNetcat, "The image used for trillian netcat.")
	utils.RelatedImageFlag("fulcio-server-image", images.FulcioServer, "The image used for the fulcio server.")
	utils.RelatedImageFlag("rekor-redis-image", images.RekorRedis, "The image used for redis.")
	utils.RelatedImageFlag("rekor-server-image", images.RekorServer, "The image used for rekor server.")
	utils.RelatedImageFlag("rekor-search-ui-image", images.RekorSearchUi, "The image used for rekor search ui.")
	utils.RelatedImageFlag("backfill-redis-image", images.BackfillRedis, "The image used for backfill redis.")
	utils.RelatedImageFlag("tuf-image", images.Tuf, "The image used for TUF.")
	utils.RelatedImageFlag("ctlog-image", images.CTLog, "The image used for ctlog.")
	utils.RelatedImageFlag("http-server-image", images.HttpServer, "The image used to serve our cli binary's.")
	utils.RelatedImageFlag("client-server-image", images.ClientServer, "The image used to serve cosign and gitsign.")
	utils.RelatedImageFlag("segment-backup-job-image", images.SegmentBackup, "The image used for the segment backup job")
	utils.RelatedImageFlag("timestamp-authority-image", images.TimestampAuthority, "The image used for Timestamp Authority")
	flag.StringVar(&clidownload.CliHostName, "cli-server-hostname", "", "The hostname for the cli server")

	klog.InitFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(klog.NewKlogr())

	// Register custom panic handlers
	utilruntime.PanicHandlers = append(utilruntime.PanicHandlers, func(r interface{}) {
		if r == http.ErrAbortHandler {
			// honor the http.ErrAbortHandler sentinel panic value:
			//   ErrAbortHandler is a sentinel panic value to abort a handler.
			//   While any panic from ServeHTTP aborts the response to the client,
			//   panicking with ErrAbortHandler also suppresses .
			return
		}

		metrics.ReconcilePanics.Inc()
	})

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		PprofBindAddress:       pprofAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f81d37df.redhat.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
		Controller: config.Controller{
			RecoverPanic: ptr.To(true),
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&securesign.SecuresignReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Securesign")
		os.Exit(1)
	}
	if err = (&fulcio.FulcioReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("fulcio-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Fulcio")
		os.Exit(1)
	}
	if err = (&trillian.TrillianReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("trillian-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Trillian")
		os.Exit(1)
	}
	if err = (&rekor.RekorReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("rekor-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Rekor")
		os.Exit(1)
	}
	if err = (&tuf.TufReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("tuf-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Tuf")
		os.Exit(1)
	}
	if err = (&ctlog.CTlogReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("ctlog-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CTlog")
		os.Exit(1)
	}
	if err = (&tsa.TimestampAuthorityReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("tsa-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TimestampAuthority")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	if err := mgr.Add(&clidownload.Component{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    setupLog.WithName("clidownload"),
	}); err != nil {
		setupLog.Error(err, "unable to set up CLIDownload component")
		os.Exit(1)
	}

	setupLog.WithName("IsOpenshift").Info(strconv.FormatBool(constants.Openshift))

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
