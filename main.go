package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

var (
	kubeconfig   string
	configPath   string
	startTimeStr string
	endTimeStr   string
	debug        bool
)

type Config struct {
	Region    string `yaml:"region"`
	ClusterID string `yaml:"clusterID"`
	Namespace string `yaml:"namespace"`
	SecretID  string `yaml:"secretID"`
	SecretKey string `yaml:"secretKey"`
}

var config Config

func main() {
	// 定义命令行参数
	flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(os.Getenv("HOME"), ".kube", "config"), "path to the kubeconfig file")
	flag.StringVar(&configPath, "config", filepath.Join(os.Getenv("HOME"), ".metrics", "config.yaml"), "path to the config file")
	flag.StringVar(&startTimeStr, "start", "2024-07-18T00:00:00+08:00", "start time for monitoring in RFC3339 format")
	flag.StringVar(&endTimeStr, "end", "2024-07-18T13:00:00+08:00", "end time for monitoring in RFC3339 format")
	flag.BoolVar(&debug, "debug", false, "show raw metrics, enabled debug logging.")

	flag.Parse()

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		klog.Fatalf("Error reading config file: %v", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		klog.Fatalf("Error unmarshaling YAML: %v", err)
	}

	// Validate the configuration
	if err := validate(config); err != nil {
		klog.Fatalf("Validation error: %v", err)
	}

	// 解析时间参数
	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		klog.Fatalf("Invalid start time: %v\n", err)
	}
	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		klog.Fatalf("Invalid end time: %v\n", err)
	}
	// 初始化Kubernetes客户端
	kc, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.Fatal(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(kc)
	if err != nil {
		klog.Fatal(err.Error())
	}

	// 获取命名空间下的所有Deployments
	deploymentsClient := clientset.AppsV1().Deployments(config.Namespace)
	deployments, err := deploymentsClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Fatal(err.Error())
	}

	// 创建CSV文件
	filename := fmt.Sprintf("deployments_metrics_%s_%s_to_%s.csv", config.Namespace, startTime.Format("20060102T150405"), endTime.Format("20060102T150405"))

	file, err := os.Create(filename)
	if err != nil {
		klog.Fatal(err.Error())
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入CSV头
	writer.Write([]string{"Namespace", "Deployment", "CPU Usage Max (percent)", "Memory Usage Max (percent)"})

	// 遍历每个Deployment
	for _, deployment := range deployments.Items {
		cpuPeakUsage, memPeakUsage := getDeploymentMetrics(deployment.Name, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
		writer.Write([]string{config.Namespace, deployment.Name, fmt.Sprintf("%f", cpuPeakUsage), fmt.Sprintf("%f", memPeakUsage)})
	}
}

func getDeploymentMetrics(deploymentName string, startTime, endTime string) (float64, float64) {
	klog.Infof("start collect %s/%s metrics.", config.Namespace, deploymentName)
	credential := common.NewCredential(
		config.SecretID,
		config.SecretKey,
	)
	// 实例化一个client选项，可选的，没有特殊需求可以跳过
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "monitor.tencentcloudapi.com"
	// 实例化要请求产品的client对象,clientProfile是可选的
	client, _ := monitor.NewClient(credential, config.Region, cpf)

	// 实例化一个请求对象,每个接口都会对应一个request对象
	request := monitor.NewDescribeStatisticDataRequest()

	request.Module = common.StringPtr("monitor")
	request.Namespace = common.StringPtr("QCE/TKE2")
	request.MetricNames = common.StringPtrs([]string{"K8sWorkloadRateCpuCoreUsedRequestMax", "K8sWorkloadRateMemWorkingSetBytesRequestMax"})
	request.Conditions = []*monitor.MidQueryCondition{
		{
			Key:      common.StringPtr("tke_cluster_instance_id"),
			Operator: common.StringPtr("="),
			Value:    common.StringPtrs([]string{config.ClusterID}),
		},
		{
			Key:      common.StringPtr("namespace"),
			Operator: common.StringPtr("="),
			Value:    common.StringPtrs([]string{config.Namespace}),
		},
		{
			Key:      common.StringPtr("workload_kind"),
			Operator: common.StringPtr("="),
			Value:    common.StringPtrs([]string{"Deployment"}),
		},
		{
			Key:      common.StringPtr("workload_name"),
			Operator: common.StringPtr("="),
			Value:    common.StringPtrs([]string{deploymentName}),
		},
	}

	request.Period = common.Uint64Ptr(3600)
	request.StartTime = common.StringPtr(startTime)
	request.EndTime = common.StringPtr(endTime)

	// 返回的resp是一个DescribeStatisticDataResponse的实例，与请求对象对应
	response, err := client.DescribeStatisticData(request)
	if _, ok := err.(*errors.TencentCloudSDKError); ok {
		klog.Warningf("An API error has returned: %s", err)
		return 0, 0
	}
	if err != nil {
		klog.Fatal(err)
	}

	if debug {
		klog.Infof("collect %s/%s raw metrics %s.", config.Namespace, deploymentName, response.ToJsonString())
	}

	metricRawData := response.Response.Data

	result := map[string]float64{
		"K8sWorkloadRateCpuCoreUsedRequestMax":        0,
		"K8sWorkloadRateMemWorkingSetBytesRequestMax": 0,
	}

	for _, metric := range metricRawData {
		if metric.MetricName == nil || len(metric.Points) == 0 || len(metric.Points[0].Values) == 0 {
			continue
		}

		maxValue := float64(0)
		for _, point := range metric.Points[0].Values {
			if point.Value != nil {
				if *point.Value > maxValue {
					maxValue = *point.Value
				}
			}
		}

		result[*metric.MetricName] = maxValue
	}

	return result["K8sWorkloadRateCpuCoreUsedRequestMax"], result["K8sWorkloadRateMemWorkingSetBytesRequestMax"]
}

func validate(config Config) error {
	if config.Region == "" {
		return fmt.Errorf("region is required")
	}
	if config.ClusterID == "" {
		return fmt.Errorf("clusterID is required")
	}
	if config.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if config.SecretID == "" {
		return fmt.Errorf("secretID is required")
	}
	if config.SecretKey == "" {
		return fmt.Errorf("secretKey is required")
	}
	return nil
}
