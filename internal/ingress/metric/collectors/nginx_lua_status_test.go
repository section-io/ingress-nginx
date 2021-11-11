/*
Copyright 2018 The Kubernetes Authors.

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

package collectors

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/ingress-nginx/internal/nginx"
)

func TestLuaStatusCollector(t *testing.T) {
	cases := []struct {
		name    string
		mock    string
		metrics []string
		want    string
	}{
		{
			name: "should return empty or zero metrics",
			mock: `
      `,
			// This has been added as a result since adding in the 'kind' label and server vs data maps
			want: `
        # HELP nginx_ingress_controller_nginx_process_cert_overflow_total number of valid items, by kind, removed forcibly when out of storage in the shared memory zone
        # TYPE nginx_ingress_controller_nginx_process_cert_overflow_total counter
        nginx_ingress_controller_nginx_process_cert_overflow_total{controller_class="nginx",controller_namespace="default",controller_pod="pod",kind="servers"} 0
        `,
			metrics: []string{
				"nginx_ingress_controller_nginx_process_dictionary_capacity",
				"nginx_ingress_controller_nginx_process_dictionary_free",
				"nginx_ingress_controller_nginx_process_cert_bytes",
				"nginx_ingress_controller_nginx_process_cert_count",
				"nginx_ingress_controller_nginx_process_cert_overflow_total",
			},
		},
		{
			name: "should return all metrics",
			mock: `
      conf_free_space_bytes: 1
      conf_capacity_bytes: 2
      cert_free_space_bytes: 3
      cert_capacity_bytes: 4
      cert_last_success: 5
      cert_last_fail: 6
      cert_last_forcible: 7
      cert_overflow_total: 8
      cert_capacity_bytes: 9
            `,
			want: `
        # HELP nginx_ingress_controller_nginx_process_cert_overflow_total number of valid items, by kind, removed forcibly when out of storage in the shared memory zone
        # TYPE nginx_ingress_controller_nginx_process_cert_overflow_total counter
        nginx_ingress_controller_nginx_process_cert_overflow_total{controller_class="nginx",controller_namespace="default",controller_pod="pod",kind="cert"} 8
        nginx_ingress_controller_nginx_process_cert_overflow_total{controller_class="nginx",controller_namespace="default",controller_pod="pod",kind="servers"} 0
        # HELP nginx_ingress_controller_nginx_process_dictionary_capacity capacity in bytes for the shm-based dictionary
        # TYPE nginx_ingress_controller_nginx_process_dictionary_capacity gauge
        nginx_ingress_controller_nginx_process_dictionary_capacity{controller_class="nginx",controller_namespace="default",controller_pod="pod",name="certificate"} 4
        nginx_ingress_controller_nginx_process_dictionary_capacity{controller_class="nginx",controller_namespace="default",controller_pod="pod",name="configuration"} 2
        # HELP nginx_ingress_controller_nginx_process_dictionary_free free page size in bytes for the shm-based dictionary
        # TYPE nginx_ingress_controller_nginx_process_dictionary_free gauge
        nginx_ingress_controller_nginx_process_dictionary_free{controller_class="nginx",controller_namespace="default",controller_pod="pod",name="certificate"} 3
        nginx_ingress_controller_nginx_process_dictionary_free{controller_class="nginx",controller_namespace="default",controller_pod="pod",name="configuration"} 1

`,
			metrics: []string{
				"nginx_ingress_controller_nginx_process_dictionary_capacity",
				"nginx_ingress_controller_nginx_process_dictionary_free",
				"nginx_ingress_controller_nginx_process_cert_bytes",
				"nginx_ingress_controller_nginx_process_cert_count",
				"nginx_ingress_controller_nginx_process_cert_overflow_total",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", nginx.StatusPort))
			if err != nil {
				t.Fatalf("creating unix listener: %s", err)
			}

			server := &httptest.Server{
				Listener: listener,
				Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)

					if r.URL.Path == "/configuration/metrics" {
						_, err := fmt.Fprintf(w, c.mock)
						if err != nil {
							t.Fatal(err)
						}

						return
					}

					fmt.Fprintf(w, "OK")
				})},
			}
			server.Start()

			time.Sleep(1 * time.Second)

			cm, err := NewNGINXLuaStatus("pod", "default", "nginx")
			if err != nil {
				t.Errorf("unexpected error creating nginx status collector: %v", err)
			}

			go cm.Start()

			reg := prometheus.NewPedanticRegistry()
			if err := reg.Register(cm); err != nil {
				t.Errorf("registering collector failed: %s", err)
			}

			if err := GatherAndCompare(cm, c.want, c.metrics, reg); err != nil {
				t.Errorf("unexpected collecting result:\n%s", err)
			}

			reg.Unregister(cm)

			server.Close()
			cm.Stop()

			listener.Close()
		})
	}
}
