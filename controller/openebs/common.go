package openebs

import (
	"io/ioutil"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/mayadata-io/openebs-operator/k8s"
	"github.com/mayadata-io/openebs-operator/types"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// setDefaultImagePullPolicyIfNotSet sets the default imagePullPolicy
// to "IfNotPresent" for all the components.
// TODO: See if this is required component wise and not at the global
// level.
func (r *Reconciler) setDefaultImagePullPolicyIfNotSet() error {
	if r.OpenEBS.Spec.ImagePullPolicy == "" {
		r.OpenEBS.Spec.ImagePullPolicy = corev1.PullIfNotPresent
	}
	return nil
}

// setDefaultStoragePathIfNotSet sets the default storage path for
// OpenEBS to "/var/openebs" if not already set.
func (r *Reconciler) setDefaultStoragePathIfNotSet() error {
	if r.OpenEBS.Spec.DefaultStoragePath == "" {
		r.OpenEBS.Spec.DefaultStoragePath = "/var/openebs"
	}
	return nil
}

// setDefaultImagePrefixIfNotSet sets the default registry prefix for
// all the container images if not already set.
func (r *Reconciler) setDefaultImagePrefixIfNotSet() error {
	if r.OpenEBS.Spec.ImagePrefix == "" {
		r.OpenEBS.Spec.ImagePrefix = "quay.io/openebs/"
	}
	return nil
}

// setDefaultStorageConfigIfNotSet sets the defaultStorageConfig value
// to "true" if not already set.
func (r *Reconciler) setDefaultStorageConfigIfNotSet() error {
	if r.OpenEBS.Spec.CreateDefaultStorageConfig == "" {
		r.OpenEBS.Spec.CreateDefaultStorageConfig = types.True
	}
	return nil
}

// BasicComponentDetails stores only the component's kind and name
type BasicComponentDetails struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// getManifests returns a mapping of component's "name_kind" to YAML of
// the respective components based on a particular version.
// Note: This method makes use of the various operator YAMLs to form this
// mapping.
func (r *Reconciler) getManifests() (map[string]string, error) {
	componentsYAMLMap := make(map[string]string)
	var yamlFile string

	switch r.OpenEBS.Spec.Version {
	case types.OpenEBSVersion150:
		yamlFile = "/templates/openebs-operator-1.5.0.yaml"
	case types.OpenEBSVersion160:
		yamlFile = "/templates/openebs-operator-1.6.0.yaml"
	default:
		return componentsYAMLMap, errors.Errorf(
			"Unsupported OpenEBS version provided, version: %+v", r.OpenEBS.Spec.Version)
	}
	data, err := ioutil.ReadFile(yamlFile)
	if err != nil {
		return componentsYAMLMap, errors.Errorf(
			"Error reading YAML file for version %s: %+v", r.OpenEBS.Spec.Version, err)
	}

	// form the mapping from component's "name_kind" as key to YAML
	// string as value using operator yaml.
	componentsYAML := strings.Split(string(data), "---")
	for _, componentYAML := range componentsYAML {
		if componentYAML == "" {
			continue
		}
		componentBasicDetails := BasicComponentDetails{}
		if err = yaml.Unmarshal([]byte(componentYAML), &componentBasicDetails); err != nil {
			return componentsYAMLMap, errors.Errorf("Error unmarshalling YAML string:%s, Error: %+v", componentYAML, err)
		}
		kind := componentBasicDetails.Kind
		name := componentBasicDetails.Name
		// Form the key using component's Name and kind separated
		// by underscore
		keyForStoringYaml := name + "_" + kind
		// Store the latest yaml of each component in a map where the key
		// is componentName_kind
		componentsYAMLMap[keyForStoringYaml] = componentYAML
	}
	return componentsYAMLMap, nil
}

// removeDisabledManifests removes the manifests which are disabled so that
// these components does not get installed.
// TODO: Delete the components if the components are disabled after installation.
func (r *Reconciler) removeDisabledManifests(manifests map[string]string) (
	map[string]string, error) {
	if strings.ToLower(r.OpenEBS.Spec.APIServer.Enabled) == types.False {
		delete(manifests, types.MayaAPIServerManifestKey)
		delete(manifests, types.MayaAPIServerServiceManifestKey)
	}
	if strings.ToLower(r.OpenEBS.Spec.Provisioner.Enabled) == types.False {
		delete(manifests, types.ProvisionerManifestKey)
	}
	if strings.ToLower(r.OpenEBS.Spec.SnapshotOperator.Enabled) == types.False {
		delete(manifests, types.SnapshotOperatorManifestKey)
	}
	if strings.ToLower(r.OpenEBS.Spec.NDM.Enabled) == types.False {
		delete(manifests, types.NDMConfigManifestKey)
		delete(manifests, types.NDMManifestKey)
	}
	if strings.ToLower(r.OpenEBS.Spec.NDMOperator.Enabled) == types.False {
		delete(manifests, types.NDMOperatorManifestKey)
	}
	if strings.ToLower(r.OpenEBS.Spec.LocalProvisioner.Enabled) == types.False {
		delete(manifests, types.LocalProvisionerManifestKey)
	}

	return manifests, nil
}

// updateManifests updates all the component's manifest as per the provided
// or the default values.
func (r *Reconciler) updateManifests(manifests map[string]string) (
	map[string]string, error) {
	var err error

	for key, value := range manifests {
		kind := strings.Split(key, "_")[1]
		switch kind {
		case types.KindNamespace:
			value, err = r.updateNamespace(value)
		case types.KindServiceAccount:
			value, err = r.updateServiceAccount(value)
		case types.KindClusterRole:
			// Note: nothing to be updated for now
			continue
			//r.updateClusterRole(value)
		case types.KindClusterRoleBinding:
			value, err = r.updateClusterRoleBinding(value)
		case types.KindDeployment:
			value, err = r.updateDeployment(value)
		case types.KindDaemonSet:
			value, err = r.updateDaemonSet(value)
		case types.KindConfigMap:
			value, err = r.updateConfigmap(value)
		case types.KindService:
			value, err = r.updateService(value)
		default:
			// Doing nothing if an unknown kind
			continue
		}
		if err != nil {
			return manifests, errors.Errorf("Error updating manifests: %+v", err)
		}
		// update manifest with the updated values
		manifests[key] = value
	}
	return manifests, nil
}

// updateDeployment updates the deployment manifest as per the given configuration.
// TODO: Make this method modular, it is a big method which seems to be doing multiple
// things.
func (r *Reconciler) updateDeployment(YAML string) (string, error) {
	var (
		replicas         *int32
		image            string
		provisionerImage string
		controllerImage  string
	)
	nodeSelector := make(map[string]string)
	deployment := &appsv1.Deployment{}
	err := yaml.Unmarshal([]byte(YAML), deployment)
	if err != nil {
		return "", errors.Errorf("Error unmarshalling deployment YAML: %+v, Error: %+v", YAML, err)
	}
	// update the namespace
	deployment.Namespace = r.OpenEBS.Namespace

	switch deployment.Name {
	case types.MayaAPIServerNameKey:
		replicas = r.OpenEBS.Spec.APIServer.Replicas
		image = r.OpenEBS.Spec.APIServer.Image

		// get desired maya-apiserver as per given configuration
		mayaAPIServer := &MayaAPIServer{
			Object: deployment,
		}
		mayaAPIServer, err = mayaAPIServer.updateManifest(r)
		if err != nil {
			return "", err
		}
		deployment = mayaAPIServer.Object
		nodeSelector = r.OpenEBS.Spec.APIServer.NodeSelector

	case types.ProvisionerNameKey:
		replicas = r.OpenEBS.Spec.Provisioner.Replicas
		image = r.OpenEBS.Spec.Provisioner.Image
		nodeSelector = r.OpenEBS.Spec.Provisioner.NodeSelector

	case types.SnapshotOperatorNameKey:
		replicas = r.OpenEBS.Spec.SnapshotOperator.Replicas
		provisionerImage = r.OpenEBS.Spec.SnapshotOperator.Provisioner.Image
		controllerImage = r.OpenEBS.Spec.SnapshotOperator.Controller.Image
		nodeSelector = r.OpenEBS.Spec.SnapshotOperator.NodeSelector

	case types.NDMOperatorNameKey:
		replicas = r.OpenEBS.Spec.NDMOperator.Replicas
		image = r.OpenEBS.Spec.NDMOperator.Image
		nodeSelector = r.OpenEBS.Spec.NDMOperator.NodeSelector

	case types.LocalProvisionerNameKey:
		replicas = r.OpenEBS.Spec.LocalProvisioner.Replicas
		image = r.OpenEBS.Spec.LocalProvisioner.Image
		nodeSelector = r.OpenEBS.Spec.LocalProvisioner.NodeSelector

	case types.AdmissionServerNameKey:
		replicas = r.OpenEBS.Spec.AdmissionServer.Replicas
		image = r.OpenEBS.Spec.AdmissionServer.Image
		nodeSelector = r.OpenEBS.Spec.AdmissionServer.NodeSelector

	}

	// update the replica count only if it is greater than 1 since the
	// default value itself is 1.
	// TODO: Validate the replica count value and throw error or take
	// some action based on that.
	if *replicas > 1 {
		deployment.Spec.Replicas = replicas
	}
	for i, container := range deployment.Spec.Template.Spec.Containers {
		container.ImagePullPolicy = r.OpenEBS.Spec.ImagePullPolicy
		// Explicitly checking for openebs-snapshot-operator in order to update
		// its multiple containers.
		// TODO: handle multiple container update cases in a better way, this seems
		// to be a very naive way.
		if deployment.Name == types.SnapshotOperatorNameKey {
			if container.Name == types.SnapshotControllerContainerKey {
				container.Image = controllerImage
			} else if container.Name == types.SnapshotProvisionerContainerKey {
				container.Image = provisionerImage
			}
		} else {
			container.Image = image
		}
		deployment.Spec.Template.Spec.Containers[i] = container
	}
	// update the nodeSelector value
	if nodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = nodeSelector
	}

	rawDeployment, err := yaml.Marshal(deployment)
	if err != nil {
		return "", errors.Errorf("Error marshalling deployment struct: %+v", err)
	}
	return string(rawDeployment), nil
}

// updateConfigmap updates the configmap manifest as per the given configuration.
func (r *Reconciler) updateConfigmap(YAML string) (string, error) {
	configmap := &corev1.ConfigMap{}
	err := yaml.Unmarshal([]byte(YAML), configmap)
	if err != nil {
		return "", errors.Errorf("Error unmarshalling configmap YAML: %+v", err)
	}
	configmap.Namespace = r.OpenEBS.Namespace

	switch configmap.Name {
	case types.NDMConfigNameKey:
		r.updateNDMConfig(configmap)
	}
	rawConfigmap, err := yaml.Marshal(configmap)
	if err != nil {
		return "", errors.Errorf("Error marshalling configmap struct: %+v", err)
	}
	return string(rawConfigmap), nil
}

// updateService updates the service manifest as per the given configuration.
func (r *Reconciler) updateService(YAML string) (string, error) {
	service := corev1.Service{}
	err := yaml.Unmarshal([]byte(YAML), &service)
	if err != nil {
		return "", errors.Errorf("Error unmarshalling service YAML: %+v", err)
	}
	service.Namespace = r.OpenEBS.Namespace

	rawService, err := yaml.Marshal(service)
	if err != nil {
		return "", errors.Errorf("Error marshalling service struct: %+v", err)
	}
	return string(rawService), nil
}

// updateDaemonSet updates the daemonset manifest as per the given configuration.
func (r *Reconciler) updateDaemonSet(YAML string) (string, error) {
	var (
		image string
	)
	daemonset := &appsv1.DaemonSet{}
	err := yaml.Unmarshal([]byte(YAML), daemonset)
	if err != nil {
		return "", errors.Errorf("Error unmarshalling daemonSet YAML: %+v", err)
	}
	daemonset.Namespace = r.OpenEBS.Namespace

	switch daemonset.Name {
	case types.NDMNameKey:
		image = r.OpenEBS.Spec.NDM.Image
		r.updateNDM(daemonset)
	}

	// update the daemonset containers with the images and imagePullPolicy
	for i, container := range daemonset.Spec.Template.Spec.Containers {
		container.ImagePullPolicy = r.OpenEBS.Spec.ImagePullPolicy
		container.Image = image

		daemonset.Spec.Template.Spec.Containers[i] = container
	}

	rawDaemonSet, err := yaml.Marshal(daemonset)
	if err != nil {
		return "", errors.Errorf("Error marshalling daemonSet struct: %+v", err)
	}
	return string(rawDaemonSet), nil
}

// deployComponents deploys all the components which is part of the
// given manifest.
func deployComponents(manifests map[string]string) error {
	var (
		err error
	)
	for key, value := range manifests {
		kind := strings.Split(key, "_")[1]
		switch kind {
		case types.KindNamespace:
			err = k8s.DeployNamespace(value)
		case types.KindServiceAccount:
			err = k8s.DeployServiceAccount(value)
		case types.KindClusterRole:
			err = k8s.DeployClusterRole(value)
		case types.KindClusterRoleBinding:
			err = k8s.DeployClusterRoleBinding(value)
		case types.KindDeployment:
			err = k8s.DeployDeployment(value)
		case types.KindDaemonSet:
			err = k8s.DeployDaemonSet(value)
		case types.KindConfigMap:
			err = k8s.DeployConfigMap(value)
		case types.KindService:
			err = k8s.DeployService(value)
		}
		if err != nil {
			return errors.Errorf("Error deploying components: %+v", err)
		}
	}
	return nil
}