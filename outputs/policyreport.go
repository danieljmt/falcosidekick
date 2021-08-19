package outputs

import (
	"context"
	"fmt"
	"log"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/falcosecurity/falcosidekick/types"
	"github.com/kubernetes-sigs/wg-policy-prototypes/policy-report/kube-bench-adapter/pkg/apis/wgpolicyk8s.io/v1alpha2"
	clusterpolicyreport "github.com/kubernetes-sigs/wg-policy-prototypes/policy-report/kube-bench-adapter/pkg/apis/wgpolicyk8s.io/v1alpha2"
	policyreport "github.com/kubernetes-sigs/wg-policy-prototypes/policy-report/kube-bench-adapter/pkg/apis/wgpolicyk8s.io/v1alpha2"
	crdClient "github.com/kubernetes-sigs/wg-policy-prototypes/policy-report/kube-bench-adapter/pkg/generated/v1alpha2/clientset/versioned"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
)

var polreport *policyreport.PolicyReport = &policyreport.PolicyReport{
	ObjectMeta: metav1.ObjectMeta{
		Name: "dummy-policy-report",
	},
	Summary: v1alpha2.PolicyReportSummary{
		Fail: 0,
		Warn: 0, //to-do
	},
}
var report *clusterpolicyreport.ClusterPolicyReport = &clusterpolicyreport.ClusterPolicyReport{
	ObjectMeta: metav1.ObjectMeta{
		Name: "dummy-cluster-policy-report",
	},
	Summary: v1alpha2.PolicyReportSummary{
		Fail: 0,
		Warn: 0, //to-do
	},
}

//in accordance with PolicyReport CRD
var failbound int
var repcount int = 0
var polrepcount int = 0

func NewPolicyReportClient(config *types.Configuration, stats *types.Statistics, promStats *types.PromStatistics, statsdClient, dogstatsdClient *statsd.Client) (*Client, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		restConfig, err = clientcmd.BuildConfigFromFlags("", config.PolicyReport.Kubeconfig)
		if err != nil {
			fmt.Printf("[ERROR] :unable to load kube config file: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	crdclient, err := crdClient.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	return &Client{
		OutputType:       "PolicyReport",
		Config:           config,
		Stats:            stats,
		PromStats:        promStats,
		StatsdClient:     statsdClient,
		DogstatsdClient:  dogstatsdClient,
		KubernetesClient: clientset,
		Crdclient:        crdclient,
	}, nil
}

// PolicyReportPost creates Policy Report Resource in Kubernetes
func (c *Client) PolicyReportCreate(falcopayload types.FalcoPayload) {
	failbound = c.Config.PolicyReport.FailThreshold
	r, namespaceScoped := newResult(falcopayload)
	if namespaceScoped == true {
		//policyreport to be created
		policyr := c.Crdclient.Wgpolicyk8sV1alpha2().PolicyReports("default")
		polrepcount++
		if polrepcount > c.Config.PolicyReport.MaxEvents {
			//To do for pruning
			checklowvalue := checklow(report.Results)
			if checklowvalue > 0 {
				polreport.Results[checklowvalue] = polreport.Results[0]
			}
			polreport.Results[0] = nil
			polreport.Results = polreport.Results[1:]
			polrepcount = polrepcount - 1
		}
		polreport.Results = append(report.Results, r)
		_, getErr := policyr.Get(context.Background(), polreport.Name, metav1.GetOptions{})
		if errors.IsNotFound(getErr) {
			result, err := policyr.Create(context.TODO(), polreport, metav1.CreateOptions{})
			if err != nil {
				log.Printf("[ERROR] : %v\n", err)
			}
			fmt.Printf("[INFO] :Created policy-report %q.\n", result.GetObjectMeta().GetName())
		} else {
			// Update existing Policy Report
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				result, err := policyr.Get(context.Background(), polreport.GetName(), metav1.GetOptions{})
				if errors.IsNotFound(err) {
					// This doesnt ever happen even if it is already deleted or not found
					log.Printf("[ERROR] :%v not found", polreport.GetName())
					return nil
				}
				if err != nil {
					return err
				}
				polreport.SetResourceVersion(result.GetResourceVersion())
				_, updateErr := policyr.Update(context.Background(), polreport, metav1.UpdateOptions{})
				return updateErr
			})
			if retryErr != nil {
				fmt.Printf("[ERROR] :update failed: %v", retryErr)
			}
			fmt.Println("[INFO] :updated policy report...")
		}
	} else {
		//clusterpolicyreport to be created
		clusterpr := c.Crdclient.Wgpolicyk8sV1alpha2().ClusterPolicyReports()
		repcount++
		if repcount > c.Config.PolicyReport.MaxEvents {
			//To do for pruning
			checklowvalue := checklow(report.Results)
			if checklowvalue > 0 {
				report.Results[checklowvalue] = report.Results[0]
			}
			report.Results[0] = nil
			report.Results = report.Results[1:]
			repcount = repcount - 1
		}
		report.Results = append(report.Results, r)
		_, getErr := clusterpr.Get(context.Background(), report.Name, metav1.GetOptions{})
		if errors.IsNotFound(getErr) {
			result, err := clusterpr.Create(context.TODO(), report, metav1.CreateOptions{})
			if err != nil {
				log.Printf("[ERROR] : %v\n", err)
			}
			fmt.Printf("[INFO] :Created cluster-policy-report %q.\n", result.GetObjectMeta().GetName())
		} else {
			// Update existing Policy Report
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				result, err := clusterpr.Get(context.Background(), report.GetName(), metav1.GetOptions{})
				if errors.IsNotFound(err) {
					// This doesnt ever happen even if it is already deleted or not found
					log.Printf("[ERROR] :%v not found", report.GetName())
					return nil
				}
				if err != nil {
					return err
				}
				report.SetResourceVersion(result.GetResourceVersion())
				_, updateErr := clusterpr.Update(context.Background(), report, metav1.UpdateOptions{})
				return updateErr
			})
			if retryErr != nil {
				fmt.Printf("[ERROR] :update failed: %v", retryErr)
			}
			fmt.Println("[INFO] :updated cluster policy report...")
		}

	}
}

//newResult creates a new entry for Reports
func newResult(FalcoPayload types.FalcoPayload) (c *clusterpolicyreport.PolicyReportResult, namespaceScoped bool) {
	namespaceScoped = false // decision variable to increment for policyreport and clusterpolicyreport //to do //false for clusterpolicyreport
	var m = make(map[string]string)
	for index, element := range FalcoPayload.OutputFields {
		if index == "ka.target.namespace" || index == "k8s.ns.name" {
			namespaceScoped = true //true for policyreport
		}
		m[index] = fmt.Sprintf("%v", element)
	}
	const PolicyReportSource string = "Falco"
	var pri string //initial hardcoded priority bounds
	if FalcoPayload.Priority > types.PriorityType(failbound) {
		if namespaceScoped == true {
			polreport.Summary.Fail++
		} else {
			report.Summary.Fail++
		}
		pri = "high"
	} else if FalcoPayload.Priority < types.PriorityType(failbound) {
		if namespaceScoped == true {
			polreport.Summary.Warn++
		} else {
			report.Summary.Warn++
		}
		pri = "low"
	} else {
		if namespaceScoped == true {
			polreport.Summary.Warn++
		} else {
			report.Summary.Warn++
		}
		pri = "medium"
	}
	return &clusterpolicyreport.PolicyReportResult{
		Policy:      FalcoPayload.Rule,
		Source:      PolicyReportSource,
		Scored:      false,
		Timestamp:   metav1.Timestamp{Seconds: int64(FalcoPayload.Time.Second()), Nanos: int32(FalcoPayload.Time.Nanosecond())},
		Severity:    v1alpha2.PolicyResultSeverity(pri),
		Result:      "fail",
		Description: FalcoPayload.Output,
		Properties:  m,
	}, namespaceScoped
}
func checklow(result []*policyreport.PolicyReportResult) (swapint int) {
	var i int
	for i = 0; i < len(result); i++ {
		if result[i].Severity == "medium" || result[i].Severity == "low" {
			return i
		}
	}
	return -1
}