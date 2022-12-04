package main

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)

	cfg, err := configLoader.ClientConfig()
	if err != nil {
		panic(err)
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err.Error())
	}

	// generate int64 pointer
	genInt64 := func(i int64) *int64 { return &i }

	// watch options
	opts := metav1.ListOptions{
		TimeoutSeconds: genInt64(120),
		Watch:          true,
	}

	// define object to watch
	namespace := "platform-csi-driver"
	api := cs.CoreV1().Events(namespace)

	// create the watcher
	watcher, err := api.Watch(context.TODO(), opts)
	if err != nil {
		panic(err)
	}

	// podEvent holds events data associated with a Pod
	type PodEvent struct {
		UID          types.UID
		PodName      string
		PodNamespace string
		// OwnerReferences []metav1.OwnerReference
		EventType      string
		Reason         string
		Message        string
		Count          int32
		FirstTimestamp time.Time
		LastTimestamp  time.Time
	}
	counter := 0
	// iterate all the events
	for event := range watcher.ResultChan() {

		item := event.Object.(*v1.Event)

		e := PodEvent{
			UID:          item.InvolvedObject.UID,
			PodName:      item.InvolvedObject.Name,
			PodNamespace: item.InvolvedObject.Namespace,
			// OwnerReferences: item.ObjectMeta.OwnerReferences,
			Reason:         item.Reason,
			EventType:      item.Type,
			Message:        item.Message,
			Count:          item.Count,
			FirstTimestamp: item.FirstTimestamp.Time,
			LastTimestamp:  item.LastTimestamp.Time,
		}

		// fmt.Printf("\n%T, %+v\n", item.ObjectMeta, item.ObjectMeta) // DEBUG

		switch item.Reason {

		case "Failed":
			// when Pod fails
			fmt.Printf("\nFailed PodEvent:\n%+v\n", e)

		case "BackOff":
			// when Pod gets into BackOff state
			fmt.Printf("\nBackOff PodEvent:\n%+v\n", e)

		// case	"NetworkNotReady":
			// this leads to "FailedCreatePodSandBox", containerd issue
		case "FailedCreatePodSandBox":
			// when Pod fails due to containerd issue
			counter += 1
			fmt.Printf("\n%dFailedCreatePodSnadbox PodEvent:\n%+v\n", counter, e)
			

			// this is where we trigger Pod deleteion login in a go routine
			// check Pod has Owner/Controller
			// check Pod is in a Pending state or has Exit Code != 0 in Container Statuses or check if it's Not Ready
			// if True, delete Pod

		default:
			// catch unknown reasons
			fmt.Printf("\nDO NOT KNOW (%s) PodEvent:\n%+v\n", item.Reason, e)
		}
	}
}
