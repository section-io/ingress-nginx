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

package collectors

import (
	"log"
	"math"
	"regexp"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/ingress-nginx/internal/nginx"
	"k8s.io/klog"
)

var (
	// these map to configuration_data dict in the lua
	reConfCapacityBytes  = regexp.MustCompile(`conf_capacity_bytes: (\d+)`)
	reConfFreeSpaceBytes = regexp.MustCompile(`conf_free_space_bytes: (\d+)`)

	// this is the size of the json payload posted from ingress-controller to lua
	reCertLastBytes = regexp.MustCompile(`cert_last_bytes: (\d+)`)

	// these map to certificate_data dict in the lua
	reCertCapacityBytes  = regexp.MustCompile(`cert_capacity_bytes: (\d+)`)
	reCertFreeSpaceBytes = regexp.MustCompile(`cert_free_space_bytes: (\d+)`)
	reCertLastFail       = regexp.MustCompile(`cert_last_fail: (\d+)`)
	reCertLastForcible   = regexp.MustCompile(`cert_last_forcible: (\d+)`)
	reCertLastSuccess    = regexp.MustCompile(`cert_last_success: (\d+)`)
	reCertOverflowTotal  = regexp.MustCompile(`cert_overflow_total: (\d+)`)

	// these map to certificate_servers dict in the lua
	reServersDictCapacityBytes  = regexp.MustCompile(`servers_capacity_bytes: (\d+)`)
	reServersDictFreeSpaceBytes = regexp.MustCompile(`servers_free_space_bytes: (\d+)`)
	reServersDictFailCount      = regexp.MustCompile(`servers_last_fail: (\d+)`)
	reServersDictForcibleCount  = regexp.MustCompile(`servers_last_forcible: (\d+)`)
	reServersDictSuccessCount   = regexp.MustCompile(`servers_last_success: (\d+)`)
	reServersOverflowTotal      = regexp.MustCompile(`servers_overflow_total: (\d+)`)
)

type (
	nginxLuaStatusCollector struct {
		scrapeChan chan scrapeRequest

		data *nginxLuaStatusData
	}

	nginxLuaStatusData struct {
		entryCount               *prometheus.Desc
		certificateLastBytes     *prometheus.Desc
		overflowTotal            *prometheus.Desc
		sharedDictionaryCapacity *prometheus.Desc
		sharedDictionaryFree     *prometheus.Desc
	}

	basicLuaStatus struct {

		// these map to conf_data in the lua
		ConfigurationCapacity  float64
		ConfigurationFreeSpace float64

		// this is the payload size of certs posted from ingress-controller to the lua
		CertificateLastBytes float64

		// these map to cert_data
		CertificateCapacity      float64
		CertificateFreeSpace     float64
		CertificateLastFail      float64
		CertificateLastForcible  float64
		CertificateLastSuccess   float64
		CertificateOverflowTotal float64

		// these map to servers_data in the lua
		ServersCapacity      float64
		ServersFreeSpace     float64
		ServersLastFail      float64
		ServersLastForcible  float64
		ServersLastSuccess   float64
		ServersOverflowTotal float64
	}
)

// NGINXLuaStatusCollector defines a LUA status collector interface
type NGINXLuaStatusCollector interface {
	prometheus.Collector

	Start()
	Stop()
}

// NewNGINXLuaStatus returns a new prometheus collector for the dynamic configuration+certificate LUA
func NewNGINXLuaStatus(podName, namespace, ingressClass string) (NGINXLuaStatusCollector, error) {
	p := nginxLuaStatusCollector{
		scrapeChan: make(chan scrapeRequest),
	}

	constLabels := prometheus.Labels{
		"controller_namespace": namespace,
		"controller_class":     ingressClass,
		"controller_pod":       podName,
	}

	// These prometheus metrics are used to surface values via the path
	// /metrics in made available by a kubernetes service for the
	// nginx-controller
	p.data = &nginxLuaStatusData{
		// nginx_ingress_controller_nginx_process_entry_count{result="{success,forcible,fail}"}
		entryCount: prometheus.NewDesc(
			prometheus.BuildFQName(PrometheusNamespace, subSystem, "entry_count"),
			"number of certs related entries in last update state result={success,forcible,fail}, kind={cert,servers}",
			[]string{"result", "kind"}, constLabels),

		// nginx_ingress_controller_nginx_process_cert_bytes{}
		certificateLastBytes: prometheus.NewDesc(
			prometheus.BuildFQName(PrometheusNamespace, subSystem, "cert_bytes"),
			"size in bytes of last configuration update",
			nil, constLabels),

		// nginx_ingress_controller_nginx_process_cert_overflow_total{kind="{cert,servers}"}
		overflowTotal: prometheus.NewDesc(
			prometheus.BuildFQName(PrometheusNamespace, subSystem, "cert_overflow_total"),
			"number of valid items, by kind, removed forcibly when out of storage in the shared memory zone",
			[]string{"kind"}, constLabels),

		// nginx_ingress_controller_nginx_process_dictionary_capacity{name="{certificate,configuration,servers}"}
		sharedDictionaryCapacity: prometheus.NewDesc(
			prometheus.BuildFQName(PrometheusNamespace, subSystem, "dictionary_capacity"),
			"capacity in bytes for the shm-based dictionary",
			[]string{"name"}, constLabels),

		// nginx_ingress_controller_nginx_process_dictionary_free{name="{certificate,configuration,servers}"}
		sharedDictionaryFree: prometheus.NewDesc(
			prometheus.BuildFQName(PrometheusNamespace, subSystem, "dictionary_free"),
			"free page size in bytes for the shm-based dictionary",
			[]string{"name"}, constLabels),
	}

	return p, nil
}

// Describe implements prometheus.Collector.
func (p nginxLuaStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- p.data.certificateLastBytes
	ch <- p.data.entryCount
	ch <- p.data.overflowTotal
	ch <- p.data.sharedDictionaryCapacity
	ch <- p.data.sharedDictionaryFree
}

// Collect implements prometheus.Collector.
func (p nginxLuaStatusCollector) Collect(ch chan<- prometheus.Metric) {
	req := scrapeRequest{results: ch, done: make(chan struct{})}
	p.scrapeChan <- req
	<-req.done
}

func (p nginxLuaStatusCollector) Start() {
	for req := range p.scrapeChan {
		ch := req.results
		p.scrape(ch)
		req.done <- struct{}{}
	}
}

func (p nginxLuaStatusCollector) Stop() {
	close(p.scrapeChan)
}

func toFloat64(data []string, pos int) float64 {
	if len(data) == 0 {
		return math.NaN()
	}
	if pos > len(data) {
		return math.NaN()
	}
	if v, err := strconv.ParseFloat(data[pos], 64); err == nil {
		return v
	}
	return math.NaN()
}

func parseLua(data string) *basicLuaStatus {
	return &basicLuaStatus{
		// these map to certificate_data dict in the loa
		CertificateCapacity:      toFloat64(reCertCapacityBytes.FindStringSubmatch(data), 1),
		CertificateFreeSpace:     toFloat64(reCertFreeSpaceBytes.FindStringSubmatch(data), 1),
		CertificateLastBytes:     toFloat64(reCertLastBytes.FindStringSubmatch(data), 1),
		CertificateLastFail:      toFloat64(reCertLastFail.FindStringSubmatch(data), 1),
		CertificateLastForcible:  toFloat64(reCertLastForcible.FindStringSubmatch(data), 1),
		CertificateLastSuccess:   toFloat64(reCertLastSuccess.FindStringSubmatch(data), 1),
		CertificateOverflowTotal: toFloat64(reCertOverflowTotal.FindStringSubmatch(data), 1),

		// these map to configuration_data dict in the loa
		ConfigurationCapacity:  toFloat64(reConfCapacityBytes.FindStringSubmatch(data), 1),
		ConfigurationFreeSpace: toFloat64(reConfFreeSpaceBytes.FindStringSubmatch(data), 1),

		// these map to certificate_servers_data dict in the lua
		ServersCapacity:     toFloat64(reServersDictCapacityBytes.FindStringSubmatch(data), 1),
		ServersFreeSpace:    toFloat64(reServersDictFreeSpaceBytes.FindStringSubmatch(data), 1),
		ServersLastFail:     toFloat64(reServersDictFailCount.FindStringSubmatch(data), 1),
		ServersLastForcible: toFloat64(reServersDictForcibleCount.FindStringSubmatch(data), 1),
		ServersLastSuccess:  toFloat64(reServersDictSuccessCount.FindStringSubmatch(data), 1),
	}
}

func mustNewConstMetricIfANumber(ch chan<- prometheus.Metric, desc *prometheus.Desc, valueType prometheus.ValueType, value float64, labelValues ...string) {
	if !math.IsNaN(value) {
		ch <- prometheus.MustNewConstMetric(desc, valueType, value, labelValues...)
	}
}

// nginxStatusCollector scrape the nginx status
func (p nginxLuaStatusCollector) scrape(ch chan<- prometheus.Metric) {
	klog.V(3).Infof("start scraping socket: %v", nginx.LuaStatusPath)
	status, data, err := nginx.NewGetStatusRequest(nginx.LuaStatusPath)
	if err != nil {
		log.Printf("%v", err)
		klog.Warningf("unexpected error obtaining nginx lua status info: %v", err)
		return
	}

	if status < 200 || status >= 400 {
		klog.Warningf("unexpected error obtaining nginx lua status info (status %v)", status)
		return
	}

	s := parseLua(string(data))

	mustNewConstMetricIfANumber(ch, p.data.certificateLastBytes, prometheus.GaugeValue, s.CertificateLastBytes)

	mustNewConstMetricIfANumber(ch, p.data.entryCount, prometheus.GaugeValue, s.CertificateLastFail, "fail", "cert")
	mustNewConstMetricIfANumber(ch, p.data.entryCount, prometheus.GaugeValue, s.CertificateLastForcible, "forcible", "cert")
	mustNewConstMetricIfANumber(ch, p.data.entryCount, prometheus.GaugeValue, s.CertificateLastSuccess, "success", "cert")

	mustNewConstMetricIfANumber(ch, p.data.entryCount, prometheus.GaugeValue, s.ServersLastFail, "fail", "servers")
	mustNewConstMetricIfANumber(ch, p.data.entryCount, prometheus.GaugeValue, s.ServersLastForcible, "forcible", "servers")
	mustNewConstMetricIfANumber(ch, p.data.entryCount, prometheus.GaugeValue, s.ServersLastSuccess, "success", "servers")

	mustNewConstMetricIfANumber(ch, p.data.overflowTotal, prometheus.CounterValue, s.CertificateOverflowTotal, "cert")
	mustNewConstMetricIfANumber(ch, p.data.overflowTotal, prometheus.CounterValue, s.ServersOverflowTotal, "servers")

	mustNewConstMetricIfANumber(ch, p.data.sharedDictionaryCapacity, prometheus.GaugeValue, s.CertificateCapacity, "certificate")
	mustNewConstMetricIfANumber(ch, p.data.sharedDictionaryCapacity, prometheus.GaugeValue, s.ConfigurationCapacity, "configuration")
	mustNewConstMetricIfANumber(ch, p.data.sharedDictionaryCapacity, prometheus.GaugeValue, s.ServersCapacity, "servers")

	mustNewConstMetricIfANumber(ch, p.data.sharedDictionaryFree, prometheus.GaugeValue, s.CertificateFreeSpace, "certificate")
	mustNewConstMetricIfANumber(ch, p.data.sharedDictionaryFree, prometheus.GaugeValue, s.ConfigurationFreeSpace, "configuration")
	mustNewConstMetricIfANumber(ch, p.data.sharedDictionaryFree, prometheus.GaugeValue, s.ServersFreeSpace, "servers")
}
