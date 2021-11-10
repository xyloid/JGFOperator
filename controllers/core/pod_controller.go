/*
Copyright 2021.

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

package core

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	fluxv1 "fluxframework.io/jgfoperator/apis/flux/v1"
	// +kubebuilder:scaffold:imports
	podinfoclientset "fluxframework.io/jgfoperator/generated/flux/clientset/versioned"
)

var (
	newScheme = runtime.NewScheme()
	setupLog  = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(newScheme)

	_ = fluxv1.AddToScheme(newScheme)
	// +kubebuilder:scaffold:scheme
}

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	podInfoMap       map[string]*fluxv1.PodInfo
	podInfoClientset *podinfoclientset.Clientset
	k8sclientset     *kubernetes.Clientset
}

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=pods/finalizers,verbs=update
//+kubebuilder:rbac:groups=flux,resources=podinfoes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=flux,resources=podinfoes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=flux,resources=podinfoes/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=nodes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	clog := log.FromContext(ctx)

	myLog := clog.WithValues("pod", req.NamespacedName)
	// your logic here
	fmt.Printf("\n\nReconcile function is called: %s\n", req.NamespacedName)

	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		myLog.Error(err, "unable to fetch Pod")

		// clean the corresponding resources if exists
		// This could happen since controller is synchronized on cluster level instead of each object level.
		outdatedPodInfo, exists := r.podInfoMap[req.Name]
		if exists {
			fmt.Printf("Try to delete podinfo %s\n", outdatedPodInfo.Name)
			err := r.podInfoClientset.FluxV1().PodInfos("default").Delete(context.TODO(), outdatedPodInfo.Name, metav1.DeleteOptions{})
			if err != nil {
				fmt.Printf("Failed to delete podinfo %s\n", outdatedPodInfo.Name)
			}
		}
		delete(r.podInfoMap, req.Name)
		return ctrl.Result{}, err
	}

	if !validatePod(pod) {
		return ctrl.Result{}, nil
	}

	node, err := r.k8sclientset.CoreV1().Nodes().Get(context.Background(), pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Println("Can not get node with name:" + pod.Spec.NodeName)
	} else {
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			fmt.Println("SKIP: Pod " + pod.Name + " is running on master node: " + pod.Spec.NodeName)
			return ctrl.Result{}, nil
		}
	}

	podInfo, exists := r.podInfoMap[pod.Name]

	if !exists {

		cpu_limit, ok := pod.Spec.Containers[0].Resources.Limits["cpu"]

		if ok {
			fmt.Printf("CPU limit %d \n", cpu_limit.Value())
		}

		cpu_request, ok := pod.Spec.Containers[0].Resources.Limits["cpu"]

		if ok {
			fmt.Printf("CPU request %d \n", cpu_request.Value())
		}

		printPodInspection(pod)

		fmt.Println("--------------")
		fmt.Println(cpu_limit.Value())
		fmt.Println(cpu_request.Value())
		fmt.Println("--------------")

		// create CR
		newPodInfo := createPodInfo(pod.Name, pod.Spec.NodeName, int(cpu_limit.Value()), int(cpu_request.Value()))
		_, err := r.podInfoClientset.FluxV1().PodInfos("default").Create(context.TODO(), newPodInfo, metav1.CreateOptions{})
		if err != nil {
			fmt.Println("Creation error")
			fmt.Println(err)
		}
		r.podInfoMap[pod.Name] = newPodInfo

	} else if podInfo.Spec.NodeName != pod.Spec.NodeName {
		fmt.Println("Node assignment changed")
		fmt.Printf("Old: %s\n", podInfo.Spec.NodeName)
		fmt.Printf("New: %s\n", pod.Spec.NodeName)
		// TODO: update CR
		// case 1: update nodename
		// case 2: update amount of resources
	} else {
		// remove completed pod
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {

			outdatedPodInfo, exists := r.podInfoMap[req.Name]
			if exists {
				fmt.Printf("Try to delete podinfo %s\n", outdatedPodInfo.Name)
				err := r.podInfoClientset.FluxV1().PodInfos("default").Delete(context.TODO(), outdatedPodInfo.Name, metav1.DeleteOptions{})
				if err != nil {
					fmt.Printf("Failed to delete podinfo %s\n", outdatedPodInfo.Name)
				}
			}
			delete(r.podInfoMap, req.Name)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func createPodInfo(podname, nodename string, cpulimit, cpurequest int) *fluxv1.PodInfo {
	return &fluxv1.PodInfo{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "flux/v1",
			Kind:       "PodInfo",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "podinfo-" + podname,
			Namespace: "default",
		},
		Spec: fluxv1.PodInfoSpec{
			PodName:    podname,
			NodeName:   nodename,
			CpuLimit:   cpulimit,
			CpuRequest: cpurequest,
		},
		Status: fluxv1.PodInfoStatus{},
	}
}

func validatePod(pod corev1.Pod) bool {
	if pod.Spec.NodeName == "" {
		fmt.Println("SKIP: empty nodename")
		return false
	}

	if pod.Spec.SchedulerName == "scheduling-plugin" {
		// the current name of kubeflux scheduler
		fmt.Println("SKIP: scheduled by kubeflux")
		return false
	}
	return true
}

func printPodInspection(pod corev1.Pod) {
	fmt.Printf("# Pod Inspection\n")
	fmt.Printf("Pod name:%s.\n", pod.Name)
	fmt.Printf("Pod scheduler name:%s.\n", pod.Spec.SchedulerName)
	fmt.Printf("Pod node name:%s.\n", pod.Spec.NodeName)
	fmt.Printf("Pod node name nominated:%s.\n", pod.Status.NominatedNodeName)
	fmt.Printf("Pod status phase: %s\n", pod.Status.Phase)
	fmt.Printf("Pod message: %s\n", pod.Status.Message)
	fmt.Printf("\n")
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Initilization information - package rest
	var (
		config *rest.Config
		err    error
	)

	kubeconfig := os.Getenv("KUBECONFIG")

	// Authentication / connection object - package tools/clientcmd
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating client: %v", err)
		os.Exit(1)
	}

	// Kubernetes client - package kubernetes
	r.k8sclientset = kubernetes.NewForConfigOrDie(config)

	// // Get pods -- package metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// pods, _ := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
	// for _, p := range pods.Items {
	// 	fmt.Println(p.GetName())
	// }

	// nodes, _ := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	// for _, n := range nodes.Items {
	// 	fmt.Println(n.GetName())
	// }

	r.podInfoMap = make(map[string]*fluxv1.PodInfo)
	kubeConfig := ctrl.GetConfigOrDie()
	r.podInfoClientset = podinfoclientset.NewForConfigOrDie(kubeConfig)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
