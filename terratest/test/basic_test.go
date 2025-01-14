package test

/*
Copyright 2022 The k8gb Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Generated by GoLic, for more details see: https://github.com/AbsaOSS/golic
*/

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBasicExample(t *testing.T) {
	t.Parallel()

	var coreDNSPods []corev1.Pod

	clientIP := ""
	// Path to the Kubernetes resource config we will test
	kubeResourcePath, err := filepath.Abs("../example/dnsendpoint.yaml")
	require.NoError(t, err)
	ttlEndpoint, err := filepath.Abs("../example/dnsendpoint_ttl.yaml")
	require.NoError(t, err)
	txtEndpoint, err := filepath.Abs("../example/dnsendpoint_txt.yaml")
	require.NoError(t, err)
	brokenEndpoint, err := filepath.Abs("../example/dnsendpoint_broken.yaml")
	require.NoError(t, err)

	// To ensure we can reuse the resource config on the same cluster to test different scenarios, we setup a unique
	// namespace for the resources for this test.
	// Note that namespaces must be lowercase.
	namespaceName := fmt.Sprintf("coredns-test-%s", strings.ToLower(random.UniqueId()))

	options := k8s.NewKubectlOptions("", "", namespaceName)
	mainNsOptions := k8s.NewKubectlOptions("", "", "coredns")
	podFilter := metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=coredns",
	}

	k8s.CreateNamespace(t, options, namespaceName)

	defer k8s.DeleteNamespace(t, options, namespaceName)

	defer k8s.KubectlDelete(t, options, kubeResourcePath)

	k8s.KubectlApply(t, options, kubeResourcePath)

	k8s.WaitUntilNumPodsCreated(t, mainNsOptions, podFilter, 1, 60, 1*time.Second)

	coreDNSPods = k8s.ListPods(t, mainNsOptions, podFilter)

	for _, pod := range coreDNSPods {
		k8s.WaitUntilPodAvailable(t, mainNsOptions, pod.Name, 60, 1*time.Second)
	}

	t.Run("Basic type A resolve", func(t *testing.T) {
		actualIP, err := DigIPs(t, "localhost", 1053, "host1.example.org", dns.TypeA, clientIP)
		require.NoError(t, err)
		assert.Contains(t, actualIP, "1.2.3.4")
	})

	// check for NODATA replay on non labeled endpoints
	t.Run("NODATA reply on non labeled endpoints", func(t *testing.T) {
		emptyIP, err := DigIPs(t, "localhost", 1053, "host3.example.org", dns.TypeA, clientIP)
		require.NoError(t, err)
		assert.NotContains(t, emptyIP, "1.2.3.4")
	})

	t.Run("Validate artificial(broken) DNS doesn't break CoreDNS", func(t *testing.T) {
		k8s.KubectlApply(t, options, brokenEndpoint)
		_, err := DigIPs(t, "localhost", 1053, "broken1.example.org", dns.TypeA, clientIP)
		require.Error(t, err)
		_, err = DigIPs(t, "localhost", 1053, "broken2.example.org", dns.TypeA, clientIP)
		require.Error(t, err)

		// We still able to get "healthy" records
		currentIP, err := DigIPs(t, "localhost", 1053, "host1.example.org", dns.TypeA, clientIP)
		require.NoError(t, err)
		assert.Contains(t, currentIP, "1.2.3.4")
	})

	t.Run("TTL is correctly evaluated", func(t *testing.T) {
		k8s.KubectlApply(t, options, ttlEndpoint)
		msg, err := DigMsg(t, "localhost", 1053, "ttl.example.org", dns.TypeA)

		require.NoError(t, err)
		assert.Equal(t, uint32(123), msg.Answer[0].(*dns.A).Hdr.Ttl)
	})

	t.Run("Type AAAA returns Rcode 0", func(t *testing.T) {
		msg, err := DigMsg(t, "localhost", 1053, "host1.example.org", dns.TypeAAAA)
		require.NoError(t, err)
		assert.Equal(t, dns.RcodeSuccess, msg.Rcode)
		assert.Equal(t, 0, len(msg.Answer))
	})
	t.Run("Type AAAA returns Rcode 3 for non existing host", func(t *testing.T) {
		msg, err := DigMsg(t, "localhost", 1053, "nonexistent.example.org", dns.TypeAAAA)
		require.NoError(t, err)
		assert.Equal(t, dns.RcodeNameError, msg.Rcode)
		assert.Equal(t, 0, len(msg.Answer))
	})
	t.Run("Basic type TXT resolve", func(t *testing.T) {
		expectedTXT := []string{"foo=bar"}
		k8s.KubectlApply(t, options, txtEndpoint)
		msg, err := DigMsg(t, "localhost", 1053, "txt.example.org", dns.TypeTXT)

		require.NoError(t, err)
		assert.Equal(t, expectedTXT, msg.Answer[0].(*dns.TXT).Txt)
	})
}
