package tests_test

import (
	"sigs.k8s.io/kustomize/v3/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/v3/k8sdeps/transformer"
	"sigs.k8s.io/kustomize/v3/pkg/fs"
	"sigs.k8s.io/kustomize/v3/pkg/loader"
	"sigs.k8s.io/kustomize/v3/pkg/plugins"
	"sigs.k8s.io/kustomize/v3/pkg/resmap"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
	"sigs.k8s.io/kustomize/v3/pkg/target"
	"sigs.k8s.io/kustomize/v3/pkg/validators"
	"testing"
)

func writeDexCrdsOverlaysIstio(th *KustTestHarness) {
	th.writeF("/manifests/dex-auth/dex-crds/overlays/istio/virtual-service.yaml", `
# This config is gated on kiali upgrade to 0.21 from 0.16 in istio 1.1.6:
# https://github.com/kiali/kiali/issues/1154
# https://github.com/istio/istio/issues/11131

apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: dex
spec:
  gateways:
  - kubeflow/kubeflow-gateway
  hosts:
  - '*'
  http:
  - match:
    - uri:
        prefix: /dex/
    route:
    - destination:
        host: dex.$(namespace).svc.cluster.local
        port:
          number: 5556
`)
	th.writeF("/manifests/dex-auth/dex-crds/overlays/istio/params.yaml", `
varReference:
- path: spec/http/route/destination/host
  kind: VirtualService
`)
	th.writeF("/manifests/dex-auth/dex-crds/overlays/istio/params.env", `
namespace=auth`)
	th.writeK("/manifests/dex-auth/dex-crds/overlays/istio", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
bases:
- ../../base
resources:
- virtual-service.yaml

configMapGenerator:
- name: dex-parameters
  behavior: merge
  env: params.env
generatorOptions:
  disableNameSuffixHash: true
vars:
- name: namespace
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.namespace
configurations:
- params.yaml
`)
	th.writeF("/manifests/dex-auth/dex-crds/base/namespace.yaml", `
apiVersion: v1
kind: Namespace
metadata:
  name: auth
`)
	th.writeF("/manifests/dex-auth/dex-crds/base/config-map.yaml", `
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: dex
data:
  config.yaml: |
    issuer: $(issuer)
    storage:
      type: kubernetes
      config:
        inCluster: true
    web:
      http: 0.0.0.0:5556
    logger:
      level: "debug"
      format: text
    oauth2:
      skipApprovalScreen: true
    enablePasswordDB: true
    staticPasswords:
    - email: $(static_email)
      hash: $(static_password_hash)
      username: $(static_username)
      userID: $(static_user_id)
    staticClients:
    - id: $(client_id)
      redirectURIs: $(oidc_redirect_uris)
      name: 'Dex Login Application'
      secret: $(application_secret)
`)
	th.writeF("/manifests/dex-auth/dex-crds/base/crds.yaml", `
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: authcodes.dex.coreos.com
spec:
  group: dex.coreos.com
  names:
    kind: AuthCode
    listKind: AuthCodeList
    plural: authcodes
    singular: authcode
  scope: Namespaced
  version: v1
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: dex
rules:
- apiGroups: ["dex.coreos.com"] # API group created by dex
  resources: ["*"]
  verbs: ["*"]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["create"] # To manage its own resources identity must be able to create customresourcedefinitions.
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: dex
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: dex
subjects:
- kind: ServiceAccount
  name: dex                 # Service account assigned to the dex pod.
  namespace: auth           # The namespace dex is running in.
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dex
  namespace: auth
`)
	th.writeF("/manifests/dex-auth/dex-crds/base/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: dex
  name: dex
spec:
  replicas: 1
  selector:
    matchLabels:
        app: dex
  template:
    metadata:
      labels:
        app: dex
    spec:
      serviceAccountName: dex
      containers:
      - image: quay.io/dexidp/dex:v2.22.0
        name: dex
        command: ["dex", "serve", "/etc/dex/cfg/config.yaml"]
        ports:
        - name: http
          containerPort: 5556
        volumeMounts:
        - name: config
          mountPath: /etc/dex/cfg
      volumes:
      - name: config
        configMap:
          name: dex
          items:
          - key: config.yaml
            path: config.yaml
`)
	th.writeF("/manifests/dex-auth/dex-crds/base/service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: dex
spec:
  type: NodePort
  ports:
  - name: dex
    port: 5556
    protocol: TCP
    targetPort: 5556
    nodePort: 32000
  selector:
    app: dex
`)
	th.writeF("/manifests/dex-auth/dex-crds/base/params.yaml", `
varReference:
- path: spec/template/spec/volumes/secret/secretName
  kind: Deployment
- path: data/config.yaml
  kind: ConfigMap
`)
	th.writeF("/manifests/dex-auth/dex-crds/base/params.env", `
# Dex Server Parameters (some params are shared with client)
dex_domain=dex.example.com
# Set issuer to https if tls is enabled
issuer=http://dex.example.com:32000
static_email=admin@example.com
static_password_hash=$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W
static_username=admin
static_user_id=08a8684b-db88-4b73-90a9-3cd1661f5466
client_id=ldapdexapp
oidc_redirect_uris=['http://login.example.org:5555/callback/onprem-cluster']
application_secret=pUBnBOY80SnXgjibTYM9ZWNzY2xreNGQok
`)
	th.writeK("/manifests/dex-auth/dex-crds/base", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: auth
resources:
- namespace.yaml
- config-map.yaml
- crds.yaml
- deployment.yaml
- service.yaml
configMapGenerator:
- name: dex-parameters
  env: params.env
generatorOptions:
  disableNameSuffixHash: true
vars:
- name: dex_domain
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.dex_domain
- name: issuer
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.issuer
- name: static_email
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.static_email
- name: static_password_hash
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.static_password_hash
- name: static_username
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.static_username
- name: static_user_id
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.static_user_id
- name: client_id
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.client_id
- name: oidc_redirect_uris
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.oidc_redirect_uris
- name: application_secret
  objref:
    kind: ConfigMap
    name: dex-parameters
    apiVersion: v1
  fieldref:
    fieldpath: data.application_secret
configurations:
- params.yaml
`)
}

func TestDexCrdsOverlaysIstio(t *testing.T) {
	th := NewKustTestHarness(t, "/manifests/dex-auth/dex-crds/overlays/istio")
	writeDexCrdsOverlaysIstio(th)
	m, err := th.makeKustTarget().MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	expected, err := m.AsYaml()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	targetPath := "../dex-auth/dex-crds/overlays/istio"
	fsys := fs.MakeRealFS()
	lrc := loader.RestrictionRootOnly
	_loader, loaderErr := loader.NewLoader(lrc, validators.MakeFakeValidator(), targetPath, fsys)
	if loaderErr != nil {
		t.Fatalf("could not load kustomize loader: %v", loaderErr)
	}
	rf := resmap.NewFactory(resource.NewFactory(kunstruct.NewKunstructuredFactoryImpl()), transformer.NewFactoryImpl())
	pc := plugins.DefaultPluginConfig()
	kt, err := target.NewKustTarget(_loader, rf, transformer.NewFactoryImpl(), plugins.NewLoader(pc, rf))
	if err != nil {
		th.t.Fatalf("Unexpected construction error %v", err)
	}
	actual, err := kt.MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	th.assertActualEqualsExpected(actual, string(expected))
}