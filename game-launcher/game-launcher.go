// Copyright 2020 Google LLC All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:generate go run agones.dev/agones/pkg/apis/agones/v1 -type Foo
package main

import (
	"context"

	// "io/ioutil"
	"crypto/tls"
	"encoding/json"
	"flag"

	// "math/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	agonesv1 "agones.dev/agones/pkg/apis/agones/v1"

	"agones.dev/agones/pkg/client/clientset/versioned"
	"agones.dev/agones/pkg/util/runtime"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/gin-gonic/gin"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
)

func init() {
	// setup logrus
	logLevel, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		logLevel = logrus.InfoLevel
	}

	logrus.SetLevel(logLevel)
	logrus.SetFormatter(&logrus.JSONFormatter{})
}

const (
	gameServerImage      = "gcr.io/agones-images/agones-ping:0.14.0"
	isHelmTest           = "IS_HELM_TEST"
	gameserversNamespace = "default"

	defaultNs = "default"
)

var home = homedir.HomeDir()
var kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")

// LoggingMiddleware is a middleware that logs all requests.
func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Log the request.
		log.Println("Received request to", c.Request.URL.Path)

		// Continue processing the request.
		c.Next()
	}
}

func main() {

	viper.AllowEmptyEnv(true)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	pflag.String(gameServerImage, "", "The Address to bind the server grpcPort to. Defaults to 'localhost'")
	viper.SetDefault(gameServerImage, gameServerImage)
	runtime.Must(viper.BindEnv(gameServerImage))

	pflag.Bool(isHelmTest, false,
		"Is Helm test - defines whether GameServer should be shut down at the end of the test or not. Defaults to false")
	viper.SetDefault(isHelmTest, false)
	runtime.Must(viper.BindEnv(isHelmTest))

	pflag.String(gameserversNamespace, defaultNs, "Namespace where GameServers are created. Defaults to default")
	viper.SetDefault(gameserversNamespace, defaultNs)
	runtime.Must(viper.BindEnv(gameserversNamespace))

	pflag.Parse()
	runtime.Must(viper.BindPFlags(pflag.CommandLine))

	// Get the port number from the environment variable.
	port := os.Getenv("PORT")
	if port == "" {
		port = "443"
	}

	router := gin.New()
	// Setup middleware
	router.Use(gin.Recovery())
	router.Use(LoggingMiddleware())
	router.LoadHTMLGlob("web/template/*")
	router.Static("/static", "./web/static")

	store := cookie.NewStore([]byte("secret"))
	router.Use(sessions.Sessions("auth-session", store))

	router.GET("/", indexHandler)
	router.GET("/rooms", roomsHandler)
	router.GET("/admin", adminHandler)
	router.POST("/create-game-server", createServerHandler)

	router.RunTLS(":443", "certs/server.crt", "certs/server.key")

}

func indexHandler(ctx *gin.Context) {
	// Generate a random number between 1 and 100.
	// randomNumber := rand.Intn(100) + 1
	session := sessions.Default(ctx)
	profile := session.Get("profile")
	if profile == nil {
		// ctx.Redirect(http.StatusSeeOther, "/")
		ctx.HTML(http.StatusOK, "index.tmpl", gin.H{
			"title": "success",
		})
	} else {
		ctx.HTML(http.StatusOK, "index.tmpl", profile)
	}

}

func adminHandler(ctx *gin.Context) {
	// Generate a random number between 1 and 100.
	// randomNumber := rand.Intn(100) + 1
	session := sessions.Default(ctx)
	profile := session.Get("profile")
	if profile == nil {
		// ctx.Redirect(http.StatusSeeOther, "/")
		ctx.HTML(http.StatusOK, "admin.tmpl", gin.H{
			"title": "success",
		})
	} else {
		ctx.HTML(http.StatusOK, "admin.tmpl", profile)
	}

}

func roomsHandler(ctx *gin.Context) {
	// Generate a random number between 1 and 100.
	// randomNumber := rand.Intn(100) + 1
	session := sessions.Default(ctx)
	profile := session.Get("profile")

	// Create a new REST client using the kubeconfig file
	// Note that we reuse the same config for both the Kubernetes Clientset and the Agones Clientset
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	logger := runtime.NewLoggerWithSource("main")
	if err != nil {
		logger.WithError(err).Fatal("Could not create in cluster config")
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// Calculate amount of pods
	pods, err := clientset.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	var podNames []string
	var podStates []string
	var CreationTimestamps []string
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, "agones-game-launcher") {
			continue
		}
		if strings.HasPrefix(pod.Name, "demo") {
			continue
		}
		if strings.HasPrefix(pod.Name, "nginx") {
			continue
		}
		// fmt.Println(pod.Status.Phase)
		podNames = append(podNames, strings.TrimPrefix(pod.Name, "helm-test-server-"))
		CreationTimestamps = append(CreationTimestamps, pod.CreationTimestamp.Format("2006-01-02 15:04:05"))
		podStates = append(podStates, string(pod.Status.Phase))
	}
	// names := []string{"John", "Paul", "George", "Ringo"}
	if profile == nil {
		// ctx.Redirect(http.StatusSeeOther, "/")
		ctx.HTML(http.StatusOK, "rooms.tmpl", gin.H{
			"title":              "Public Rooms",
			"names":              podNames,
			"CreationTimestamps": CreationTimestamps,
			"podStates":          podStates,
		})
	} else {
		ctx.HTML(http.StatusOK, "rooms.tmpl", gin.H{
			"nickname":           profile.(map[string]interface{})["nickname"],
			"names":              podNames,
			"CreationTimestamps": CreationTimestamps,
			"podStates":          podStates,
		})
	}

}

type sever struct {
	Name string `json:"name"`
}

func createServerHandler(ctx *gin.Context) {

	var createGame sever
	fmt.Println("bird activated")

	// Call BindJSON to bind the received JSON to
	// newAlbum.
	if err := ctx.BindJSON(&createGame); err != nil {
		fmt.Println("Error: %v", err)

		return
	}

	// Print Game name
	fmt.Println("Creating game server with name: ", createGame.Name)

	if createGame.Name == "Test" {
		fmt.Println("BINGO")
		//		gameServerImage := "agones-simple-app:latest"
		viper.SetDefault(gameServerImage, "agones-simple-app:latest")

	} else if createGame.Name == "Chat Room" {
		viper.SetDefault(gameServerImage, "go-chatroom-app:latest")
	} else if createGame.Name == "Adventure Game" {
		viper.SetDefault(gameServerImage, "nodejs-adventure-game:latest")
	} else if createGame.Name == "Guess the Number" {
		viper.SetDefault(gameServerImage, "nodejs-guess-game:latest")
	} else if createGame.Name == "Go ping" {
		viper.SetDefault(gameServerImage, "go-websocket-ping:latest")
	} else if createGame.Name == "Start Quest" {
		viper.SetDefault(gameServerImage, "go-quest-game:latest")
	} else {
		fmt.Println("No game name")
	}

	// Create a new REST client using the kubeconfig file
	// Note that we reuse the same config for both the Kubernetes Clientset and the Agones Clientset
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	logger := runtime.NewLoggerWithSource("main")
	if err != nil {
		logger.WithError(err).Fatal("Could not create in cluster config")
	}

	// Access to the Agones resources through the Agones Clientset
	// Note that we reuse the same config as we used for the Kubernetes Clientset
	agonesClient, err := versioned.NewForConfig(config)
	if err != nil {
		fmt.Printf("no clientset")
		logger.WithError(err).Fatal("Could not create the agones api clientset")
	}

	// Create a GameServer
	gs := &agonesv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "helm-test-server-",
			Namespace:    viper.GetString(gameserversNamespace),
		},
		Spec: agonesv1.GameServerSpec{
			Container: "simple-game-server",
			Ports: []agonesv1.GameServerPort{{
				ContainerPort: 443,
				Name:          "gameport",
				PortPolicy:    agonesv1.Dynamic,
				Protocol:      corev1.ProtocolTCP,
			}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "simple-game-server",
							Image:           viper.GetString(gameServerImage),
							ImagePullPolicy: "IfNotPresent",
						},
					},
				},
			},
		},
	}
	fmt.Printf("creating server")
	// Create a new Game Server.
	// createGameServer(ctx, gs, agonesClient, logger)

	newGS, err := agonesClient.AgonesV1().GameServers(gs.Namespace).Create(ctx, gs, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("no server")
		logrus.Fatalf("Unable to create GameServer: %v", err)
	}

	// Wait for newGS to complete
	for {
		checkGs, err := agonesClient.AgonesV1().GameServers(gs.Namespace).Get(ctx, newGS.ObjectMeta.Name, metav1.GetOptions{})

		if err != nil {
			logrus.WithError(err).Warn("error retrieving gameserver")
			continue
		}

		state := agonesv1.GameServerStateReady
		logger.WithField("gs", checkGs.ObjectMeta.Name).
			WithField("currentState", checkGs.Status.State).
			WithField("awaitingState", state).Info("Waiting for states to match")

		if checkGs.Status.State == state {
			break
		}

		time.Sleep(1 * time.Second)
	}

	gameServer, err := agonesClient.AgonesV1().GameServers(gs.Namespace).Get(ctx, newGS.Name, metav1.GetOptions{})
	if err != nil {
		fmt.Println("Error updating gameserver: %v", err)
	}

	// ToDo: This is a very complex code to get port
	gamePort := ""
	mapString := make(map[string]string)
	for key, value := range gameServer.Status.Ports {
		strKey := fmt.Sprintf("%v", key)
		strValue := fmt.Sprintf("%v", value)
		mapString[strKey] = strValue

		gamePort = strValue
	}
	gamePort = strings.TrimPrefix(strings.Trim(gamePort, "{}"), "gameport ")

	logrus.Infof("New GameServer name is: %s. Image is: %s. Address is: %s:%s", newGS.ObjectMeta.Name, viper.GetString(gameServerImage), gameServer.Status.Address, gamePort)
	// w.Write([]byte(gamePort))
	pathPartByte := strings.TrimPrefix(strings.Trim(newGS.ObjectMeta.Name, "{}"), "helm-test-server-")
	pathPartjsonStr, err := json.Marshal(pathPartByte)
	if err != nil {
		fmt.Println(err)
	}
	logrus.Infof("something like port %s and code %s", gamePort, pathPartjsonStr)

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// Calculate amount of pods
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	logrus.Infof("There are %d pods in the cluster\n", len(pods.Items))

	// Expose newGS.ObjectMeta.Name

	namespace := "default"
	pod := newGS.ObjectMeta.Name
	_, err = clientset.CoreV1().Pods(namespace).Get(context.TODO(), pod, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		fmt.Printf("Pod %s in namespace %s not found\n", pod, namespace)
	} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
		fmt.Printf("Error getting pod %s in namespace %s: %v\n",
			pod, namespace, statusError.ErrStatus.Message)
	} else if err != nil {
		panic(err.Error())
	} else {
		fmt.Printf("Found pod %s in namespace %s\n", pod, namespace)
		// Create a new Service manifest.
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
				Selector: map[string]string{
					// "app": pod,
					"agones.dev/gameserver":    pod,
					"agones.dev/role":          "gameserver",
					"agones.dev/safe-to-evict": "false",
				},
				Ports: []corev1.ServicePort{
					{
						Name: "http",
						Port: 443,
					},
				},
			},
		}
		// Create the Service.
		_, err = clientset.CoreV1().Services("default").Create(context.Background(), service, metav1.CreateOptions{})
		if err != nil {
			// TODO: Handle error.
			fmt.Print(err)
		} else {

			classname := "nginx"
			pathTypeImplementationSpecific := networkingv1.PathTypeImplementationSpecific
			pathPart := strings.Split(pod, "-")

			// Create a new Ingress manifest.
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: pod,
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
					},
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: &classname,
					Rules: []networkingv1.IngressRule{
						{
							Host: "vrvsvr.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/" + pathPart[len(pathPart)-1],
											PathType: &pathTypeImplementationSpecific,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: pod,
													Port: networkingv1.ServiceBackendPort{
														Number: 443,
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Host: "vrvsvr.io",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/" + pathPart[len(pathPart)-1],
											PathType: &pathTypeImplementationSpecific,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: pod,
													Port: networkingv1.ServiceBackendPort{
														Number: 443,
													},
												},
											},
										},
									},
								},
							},
						},
						{
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/" + pathPart[len(pathPart)-1],
											PathType: &pathTypeImplementationSpecific,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: pod,
													Port: networkingv1.ServiceBackendPort{
														Number: 443,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			// Create the Ingress.
			_, err = clientset.NetworkingV1().Ingresses("default").Create(context.Background(), ingress, metav1.CreateOptions{})
			if err != nil {
				// TODO: Handle error.
				fmt.Print(err)
			}

			// Print the Ingress name.
			fmt.Println(ingress.Name)

		}

		// Print the Service name.
		fmt.Println(service.Name)

	}

	for {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		requestURL := fmt.Sprintf("https://192.168.88.205/%s", pathPartByte)
		res, err := http.Get(requestURL)
		if err != nil {
			fmt.Printf("error making http request: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("client: status code: %d\n", res.StatusCode)
		if res.StatusCode == 200 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	ctx.JSON(200, string(pathPartjsonStr)[1:len(pathPartjsonStr)-1])

}
