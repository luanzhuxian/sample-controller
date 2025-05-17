/*
Copyright 2017 The Kubernetes Authors.

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
	"flag"
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/sample-controller/pkg/signals"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	clientset "k8s.io/sample-controller/pkg/generated/clientset/versioned"
	informers "k8s.io/sample-controller/pkg/generated/informers/externalversions"
)

// masterURL 和 kubeconfig 是支持命令行传参的变量。
// 如果程序运行在集群外（比如你的本机），就需要 kubeconfig。
// 如果在集群内部跑（比如 Pod 里），这两个可以留空。
var (
	masterURL  string
	kubeconfig string
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	// set up signals so we handle the shutdown signal gracefully
	ctx := signals.SetupSignalHandler()
	logger := klog.FromContext(ctx)

	// 根据 masterURL 和 kubeconfig 生成 Kubernetes 访问配置。
	// 如果 masterURL 和 kubeconfig 都是空字符串，BuildConfigFromFlags 里面会进一步调用：rest.InClusterConfig()（即使用 Pod 内置的环境变量、ServiceAccount Token 来连接 Kubernetes API）
	// 否则，如果传了 kubeconfig，它就读 kubeconfig 文件里的配置。
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		logger.Error(err, "Error building kubeconfig")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	// 这两个客户端都是用来和 Kubernetes API Server 进行 HTTP 通信的，区别在于它们操作的资源不同：
	// 1. kubeClient 操作的是 Kubernetes 原生资源，比如 Deployment、Service、Pod 等。
	// 2. exampleClient 操作的是自定义资源，用来访问自定义的 CRD（CustomResourceDefinition），比如 Foo。

	// 创建 Kubernetes 客户端
	// 根据传入的 kubeconfig 文件和 master 地址，生成 Kubernetes 访问配置。
	// 如果跑在 Pod 里，可以用默认的 in-cluster 配置。
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Error(err, "Error building kubernetes clientset")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	// 创建自定义资源的客户端
	// 这里的 clientset 是 sample-controller 生成的客户端，用来访问自己定义的 CRD，比如 Foo 资源。
	// 这个客户端专门知道你定义的资源结构和版本，能用来：Create / Update / Get / Delete 自定义资源。
	exampleClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Error(err, "Error building kubernetes clientset")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	// 创建 Kubernetes 的 InformerFactory，用于创建和管理 Informer。
	// SharedInformerFactory 负责管理和复用各种资源的 informer，监听资源变化（比如 Deployment 变化）并缓存到本地。
	// 这里的 kubeInformerFactory 是 Kubernetes 资源的 InformerFactory，用于创建和管理 Kubernetes 资源的 Informer。
	// 这里的 exampleInformerFactory 是自定义资源的 InformerFactory，用于创建和管理自定义资源的 Informer。
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	exampleInformerFactory := informers.NewSharedInformerFactory(exampleClient, time.Second*30)

	// 创建一个 Controller 实例，传入要监听的资源。
	// 这里控制器监听了 Deployment 和 Foo 两种资源的变化。
	controller := NewController(ctx, kubeClient, exampleClient,
		kubeInformerFactory.Apps().V1().Deployments(),
		exampleInformerFactory.Samplecontroller().V1alpha1().Foos())

	// 启动 InformerFactory，它们内部会建立 Watch，实时监听资源变化。
	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(ctx.done())
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	kubeInformerFactory.Start(ctx.Done())
	exampleInformerFactory.Start(ctx.Done())

	// 启动控制器，开始处理资源变化。
	// 这里会开启 2 个 worker 线程并发来处理资源变化。
	if err = controller.Run(ctx, 2); err != nil {
		logger.Error(err, "Error running controller")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
}

// 初始化命令行参数，在程序启动时注册命令行参数，设置 --kubeconfig 和 --master。
func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}
