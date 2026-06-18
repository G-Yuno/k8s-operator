package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	esl "github.com/ning1875/errgroup-signal/signal"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"yuno.org/nodegroup/agent"
)

var (
	kubeconfig                                 string
	dpkgStatusFilepath                         string
	procSysPath                                string
	containerHostRootPrefix                    string
	syncPeriodSeconds, k8sClientTimeOutSeconds int
)

// 定义一个获取本机ip地址的函数
func GetLocalIp() string {
	// udp 是没有连接状态，开销比较低
	// 5元组 源目的地址 ip 端口 4个+1协议 udp
	// 我去连接外部访问不到的ip地址，本机会选择路由，选择一条能出去的
	// 目的在多ip 多网卡的机器上能找到  可以出去的地址
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		fmt.Printf("net.dial.err:%v\n", err)
		return ""
	}
	defer conn.Close()
	// 正常的  192.168.0.31:51573
	localAddr := conn.LocalAddr().String()
	return strings.Split(localAddr, ":")[0]
}

func main() {

	//flag.StringVar(&kubeconfig, "kubeconfig", "./01.config", "k8s集群的config")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "k8s集群的config")
	flag.StringVar(&dpkgStatusFilepath, "dpkgStatusFilepath", "/var/lib/dpkg/status", "dpkg状态文件路径")
	flag.StringVar(&procSysPath, "procSysPath", "/proc/sys", "内核参数的proc路径")
	flag.StringVar(&containerHostRootPrefix, "containerHostRootPrefix", "", "根文件系统的根，空代表虚拟机")
	flag.IntVar(&syncPeriodSeconds, "syncPeriodSeconds", 10, "轮询同步周期")
	flag.IntVar(&k8sClientTimeOutSeconds, "k8sClientTimeOutSeconds", 3, "操作k8s集群的超时时间")
	flag.Parse()

	// 获取宿主机ip
	hostIp := os.Getenv("MY_NODE_IP")
	if hostIp == "" {
		hostIp = GetLocalIp()
	}
	//TODO 本地windows mock hostIp = "192.168.0.121"

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		klog.ErrorS(err, "clientcmd.BuildConfigFromFlags.err",
			"kubeconfig", kubeconfig,
			"hostIp", hostIp,
		)
		panic(err)
	}
	// 02 先用config对象生成clientset 操作k8s集群的客户端

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	klog.Info("初始化k8s-dynamicClient客户端成功....")

	group, stopChan := esl.SetupStopSignalContext()
	ctxAll, cancelAll := context.WithCancel(context.Background())

	// 初始化管理
	nm := agent.NewNodeConfigManager(
		dpkgStatusFilepath,
		hostIp,
		procSysPath,
		containerHostRootPrefix,
		syncPeriodSeconds,
		k8sClientTimeOutSeconds,
		dynamicClient,
	)

	// 首先添加一个退出信号管理

	group.Go(func() error {
		klog.Info("[stopchan监听启动]")
		for {
			select {
			case <-stopChan:
				klog.Info("捕获退出信号 停止ctx 通知所有任务退出")
				cancelAll()
				return nil
			}

		}
	})

	group.Go(func() error {

		klog.InfoS("计划任务--节点配置监听-缓存启动")
		err := nm.StartConfigManager(ctxAll)
		if err != nil {
			klog.ErrorS(err, "计划任务--节点配置监听--报错")
		}
		return err
	})
	group.Wait()
}
