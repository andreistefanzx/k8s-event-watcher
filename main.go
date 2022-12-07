package main

import (
	"context"
	"flag"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	apiWatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/watch"
	"log"
	"time"
)

type eventWatcher struct {
	client      *kubernetes.Clientset
	timeoutSecs int64
	namespace   string
}

// newWatcher returns a new eventWatcher object that implements cache.Watcher
func newWatcher(clientSet *kubernetes.Clientset, timeout int64, namespace string) cache.Watcher {
	log.Println("eventWatcher Initialised")
	return &eventWatcher{clientSet, timeout, namespace}
}

// Watch begins a watch on Events resources
func (w *eventWatcher) Watch(options metav1.ListOptions) (apiWatch.Interface, error) {
	log.Println("New Watcher Deployed")
	return w.client.CoreV1().Events(w.namespace).Watch(context.Background(), metav1.ListOptions{TimeoutSeconds: &w.timeoutSecs})
}

// verify eventWatcher implements cache.Watcher interface
var _ cache.Watcher = (*eventWatcher)(nil)

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

var (
	namespace  string
	dryRunMode bool
	timeout    int64 = 60
)

func main() {

	// define and parse cli params
	flag.StringVar(&namespace, "namespace", "", "kubernetes namespace to watch Events")
	flag.BoolVar(&dryRunMode, "dry-run", false, "enable dry-run mode (no changes are made, only logged")
	flag.Parse()

	// load kubeconfig
	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)

	cfg, err := configLoader.ClientConfig()
	if err != nil {
		panic(err)
	}

	// create the clientset
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	// create the cache.Watcher
	watcher := newWatcher(clientSet, timeout, namespace)

	// create a RetryWatcher using initial version "1"
	rw, err := watch.NewRetryWatcher("1", watcher)
	if err != nil {
		panic(err)
	}

	// monitor Kubernetes Events
	log.Printf("Start watching Events in namespace %s", namespace)
	for {
		// grab the event object
		event, ok := <-rw.ResultChan()
		if !ok {
			panic(fmt.Errorf("closed channel"))
		}

		// cast to Event
		item, ok := event.Object.(*corev1.Event)
		if !ok {
			panic(fmt.Errorf("invalid type '%T'", event.Object))
		}

		// collect important Event data in PodEvent
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

		// log.Printf("\n%T, %+v\n", item.ObjectMeta, item.ObjectMeta) // DEBUG

		creationTime := item.GetCreationTimestamp().Time
		msg := fmt.Sprintf(
			"%s: %s %s/%s %s",
			e.EventType,
			e.Reason,
			e.PodNamespace,
			e.PodName,
			creationTime.Format(time.RFC3339),
		)
		// skip events older then five minutes
		fiveMinsAgo := time.Now().Add(-5 * time.Minute)
		if event.Type == apiWatch.Added && creationTime.Before(fiveMinsAgo) {
			log.Println("skipping old event", msg) // DEBUG
			continue
		}

		// we're not really interested in Normal Events (?)
		if e.EventType != "Normal" {

			switch item.Reason {

			case "Failed":
				// when Pod fails
				log.Printf("%s\n%+v", msg, e)

			case "BackOff":
				// when Pod gets into BackOff state and keeps restarting
				log.Printf("%s\n%+v", msg, e)

			case "NetworkNotReady":
				// this leads to "FailedCreatePodSandBox", containerd issue
				log.Printf("%s\n%+v", msg, e)
			case "FailedCreatePodSandBox":
				// when Pod fails due to containerd issue
				log.Printf("%s\n%+v", msg, e)
				// delete Pod (only if we're not in -n/--dry-run mode)
				if !dryRunMode {
					log.Printf("Would have deleted pod %s/%s", e.PodNamespace, e.PodName)
				} else {
					log.Printf("[DRY-RUN]: Would have deleted pod %s/%s", e.PodNamespace, e.PodName)
				}

				// this is where we trigger Pod deleteion login in a go routine
				// check Pod has Owner/Controller
				// check Pod is in a Pending state or has Exit Code != 0 in Container Statuses or check if it's Not Ready
				// if True, delete Pod (BANG BANG)

			default:
				// catch unknown reasons
				log.Printf("%s\n%+v", msg, e)
			}
		} else {
			// print some info about Normal events
			// we should skip this in prod version
			log.Println(msg)
		}

		// watch events every n seconds
		time.Sleep(5 * time.Second)
	}
}
