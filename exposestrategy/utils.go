package exposestrategy

import (
	"bytes"
	"net"
	"strings"
	"text/template"

	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"k8s.io/api/core/v1"
	client "k8s.io/client-go/kubernetes"
)

func findHttpProtocol(svc *v1.Service, hostName string) string {
	// default to http
	protocol := "http"

	// if a port is on the hostname check is its a default http / https port
	_, port, err := net.SplitHostPort(hostName)
	if err == nil {
		if port == "443" || port == "8443" {
			protocol = "https"
		}
	}
	// check if the service port has a name of https
	for _, port := range svc.Spec.Ports {
		if port.Name == "https" {
			protocol = port.Name
		}
	}
	return protocol
}

func addServiceAnnotation(svc *v1.Service, hostName string) (*v1.Service, error) {
	protocol := findHttpProtocol(svc, hostName)
	return addServiceAnnotationWithProtocol(svc, hostName, protocol)
}

func addServiceAnnotationWithProtocol(svc *v1.Service, hostName string, protocol string) (*v1.Service, error) {
	if svc.Annotations == nil {
		svc.Annotations = map[string]string{}
	}

	exposeURL := protocol + "://" + hostName
	path := svc.Annotations[ApiServicePathAnnotationKey]
	if len(path) > 0 {
		exposeURL = urlJoin(exposeURL, path)
	}
	svc.Annotations[ExposeAnnotationKey] = exposeURL

	if key := svc.Annotations[ExposeHostNameAsAnnotationKey]; len(key) > 0 {
		svc.Annotations[key] = hostName
	}

	return svc, nil
}

// urlJoin joins the given URL paths so that there is a / separating them but not a double //
func urlJoin(repo string, path string) string {
	return strings.TrimSuffix(repo, "/") + "/" + strings.TrimPrefix(path, "/")
}

func removeServiceAnnotation(svc *v1.Service) *v1.Service {
	delete(svc.Annotations, ExposeAnnotationKey)
	if key := svc.Annotations[ExposeHostNameAsAnnotationKey]; len(key) > 0 {
		delete(svc.Annotations, key)
	}

	return svc
}

func createPatch(a runtime.Object, b runtime.Object, encoder runtime.Encoder, dataStruct interface{}) ([]byte, error) {
	var aBuf bytes.Buffer
	err := encoder.Encode(a, &aBuf)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to encode object: %v", a)
	}
	var bBuf bytes.Buffer
	err = encoder.Encode(b, &bBuf)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to encode object: %v", b)
	}

	aBytes := aBuf.Bytes()
	bBytes := bBuf.Bytes()

	if bytes.Compare(aBytes, bBytes) == 0 {
		return nil, nil
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(aBytes, bBytes, dataStruct)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create patch")
	}

	return patch, nil
}

type masterType string

const (
	openShift  masterType = "OpenShift"
	kubernetes masterType = "Kubernetes"
)

func typeOfMaster(c *client.Clientset) (masterType, error) {
	return kubernetes, nil
}

type urlTemplateParts struct {
	Service   string
	Namespace string
	Domain    string
}

func getURLFormat(urltemplate string) (string, error) {
	if urltemplate == "" {
		urltemplate = "{{.Service}}.{{.Namespace}}.{{.Domain}}"
	}
	placeholders := urlTemplateParts{"%[1]s", "%[2]s", "%[3]s"}
	tmpl, err := template.New("format").Parse(urltemplate)
	if err != nil {
		errors.Wrap(err, "Failed to parse UrlTemplate")
	}
	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, placeholders)
	if err != nil {
		errors.Wrap(err, "Failed to execute UrlTemplate")
	}
	return buffer.String(), nil
}

// UrlJoin joins the given paths so that there is only ever one '/' character between the paths
func UrlJoin(paths ...string) string {
	var buffer bytes.Buffer
	last := len(paths) - 1
	for i, path := range paths {
		p := path
		if i > 0 {
			buffer.WriteString("/")
			p = strings.TrimPrefix(p, "/")
		}
		if i < last {
			p = strings.TrimSuffix(p, "/")
		}
		buffer.WriteString(p)
	}
	return buffer.String()
}
