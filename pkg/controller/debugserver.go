package controller

import (
	"encoding/json"
	"github.com/AliyunContainerService/kubernetes-cronhpa-controller/pkg/server"
	"github.com/gorilla/mux"
	"html/template"
	"k8s.io/klog/v2"
	"net/http"
	"time"
)

type WebServer struct {
	cronManager *CronManager
}

func (ws *WebServer) serve() {
	r := mux.NewRouter()
	r.HandleFunc("/api.json", ws.handleJobsController)
	r.HandleFunc("/index.html", ws.handleIndexController)
	http.Handle("/", r)

	srv := &http.Server{
		Handler: r,
		Addr:    ":8000",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	klog.Fatal(srv.ListenAndServe())
}

type data struct {
	Items []Item
}

type Item struct {
	Id        string
	Name      string
	CronHPA   string
	Namespace string
	Pre       string
	Next      string
}

func (ws *WebServer) handleIndexController(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, _ := template.New("index").Parse(server.Template)
	entries := ws.cronManager.cronExecutor.ListEntries()
	d := data{
		Items: make([]Item, 0),
	}
	for _, e := range entries {
		job, ok := e.Job.(CronJob)
		if !ok {
			klog.Warningf("Failed to parse cronjob %v to web console", e)
			continue
		}
		d.Items = append(d.Items, Item{
			Id:        job.ID(),
			Name:      job.Name(),
			CronHPA:   job.CronHPAMeta().Name,
			Namespace: job.CronHPAMeta().Namespace,
			Pre:       e.Prev.String(),
			Next:      e.Next.String(),
		})
	}
	tmpl.Execute(w, d)
}

func (ws *WebServer) handleJobsController(w http.ResponseWriter, r *http.Request) {
	b, err := json.Marshal(ws.cronManager.cronExecutor.ListEntries())
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(b)
}

func NewWebServer(c *CronManager) *WebServer {
	return &WebServer{
		cronManager: c,
	}
}
