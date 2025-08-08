package driver

import (
	"fmt"
	"io/ioutil"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/log"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

var legacyDefaultImages = [...]string{
	defaultImage,
	"ubuntu-18.04",
	"ubuntu-16.04",
	"debian-9",
}

func isDefaultImageName(imageName string) bool {
	for _, defaultImage := range legacyDefaultImages {
		if imageName == defaultImage {
			return true
		}
	}
	return false
}

func (d *Driver) setImageArch(arch string) error {
	switch arch {
	case "":
		d.ImageArch = emptyImageArchitecture
	case string(hcloud.ArchitectureARM):
		d.ImageArch = hcloud.ArchitectureARM
	case string(hcloud.ArchitectureX86):
		d.ImageArch = hcloud.ArchitectureX86
	default:
		return fmt.Errorf("unknown architecture %v", arch)
	}
	return nil
}

func (d *Driver) verifyImageFlags() error {
	if d.ImageID != 0 && d.Image != "" && !isDefaultImageName(d.Image) /* support legacy behaviour */ {
		return d.flagFailure("--%v and --%v are mutually exclusive", flagImage, flagImageID)
	} else if d.ImageID != 0 && d.ImageArch != "" {
		return d.flagFailure("--%v and --%v are mutually exclusive", flagImageArch, flagImageID)
	} else if d.ImageID == 0 && d.Image == "" {
		d.Image = defaultImage
	}
	return nil
}

func (d *Driver) verifyNetworkFlags() error {
	if d.DisablePublic4 && d.DisablePublic6 && !d.UsePrivateNetwork {
		return d.flagFailure("--%v must be used if public networking is disabled (hint: implicitly set by --%v)",
			flagUsePrivateNetwork, flagDisablePublic)
	}

	if d.DisablePublic4 && d.PrimaryIPv4 != "" {
		return d.flagFailure("--%v and --%v are mutually exclusive", flagPrimary4, flagDisablePublic4)
	}

	if d.DisablePublic6 && d.PrimaryIPv6 != "" {
		return d.flagFailure("--%v and --%v are mutually exclusive", flagPrimary6, flagDisablePublic6)
	}
	return nil
}

func (d *Driver) deprecatedBooleanFlag(opts drivers.DriverOptions, flag, deprecatedFlag string) bool {
	if opts.Bool(deprecatedFlag) {
		log.Warnf("--%v is DEPRECATED FOR REMOVAL, use --%v instead", deprecatedFlag, flag)
		d.usesDfr = true
		return true
	}
	return opts.Bool(flag)
}

// mergeYAMLDocs merges two YAML documents, merging arrays under the same key.
func mergeYAMLDocs(doc1, doc2 string) (string, error) {
	var m1, m2 map[string]interface{}

	if err := yaml.Unmarshal([]byte(doc1), &m1); err != nil {
		return "", fmt.Errorf("failed to unmarshal first YAML: %w", err)
	}
	if err := yaml.Unmarshal([]byte(doc2), &m2); err != nil {
		return "", fmt.Errorf("failed to unmarshal second YAML: %w", err)
	}

	merged := mergeMaps(m1, m2)

	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged YAML: %w", err)
	}

	result := string(out)
	cloudConfigComment := "#cloud-config"
	if !strings.HasPrefix(strings.TrimSpace(result), cloudConfigComment) {
		result = cloudConfigComment + "\n" + result
	}
	return result, nil
}

// mergeMaps recursively merges src into dst, merging arrays under the same key.
func mergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		if vMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := dst[k].(map[string]interface{}); ok {
				dst[k] = mergeMaps(dstMap, vMap)
			} else {
				dst[k] = vMap
			}
		} else if vArr, ok := v.([]interface{}); ok {
			if dstArr, ok := dst[k].([]interface{}); ok {
				dst[k] = append(dstArr, vArr...)
			} else {
				dst[k] = vArr
			}
		} else {
			dst[k] = v
		}
	}
	return dst
}

func (d *Driver) setUserDataFlags(opts drivers.DriverOptions) error {
	userData := opts.String(flagUserData)
	userDataFile := opts.String(flagUserDataFile)
	additionalUserData := opts.String(flagAdditionalUserData)

	if opts.Bool(legacyFlagUserDataFromFile) {
		if userDataFile != "" {
			return d.flagFailure("--%v and --%v are mutually exclusive", flagUserDataFile, legacyFlagUserDataFromFile)
		}

		// log.Warnf("--%v is DEPRECATED FOR REMOVAL, pass '--%v \"%v\"'", legacyFlagUserDataFromFile, flagUserDataFile, userData)
		if additionalUserData != "" {
			// Read user data from file
			content, err := ioutil.ReadFile(userData)
			if err != nil {
				return err
			}
			merged, err := mergeYAMLDocs(strings.ReplaceAll(additionalUserData, `\n`, "\n"), string(content))
			if err != nil {
				return fmt.Errorf("failed to merge user data YAML: %w", err)
			}
			d.userData = merged
		} else {
			d.usesDfr = true
			d.userDataFile = userData
		}
		return nil
	}

	d.userData = userData
	d.userDataFile = userDataFile

	if d.userData != "" && d.userDataFile != "" {
		return d.flagFailure("--%v and --%v are mutually exclusive", flagUserData, flagUserDataFile)
	}

	return nil
}

func (d *Driver) setLabelsFromFlags(opts drivers.DriverOptions) error {
	d.ServerLabels = make(map[string]string)
	for _, label := range opts.StringSlice(flagServerLabel) {
		split := strings.SplitN(label, "=", 2)
		if len(split) != 2 {
			return d.flagFailure("server label %v is not in key=value format", label)
		}
		d.ServerLabels[split[0]] = split[1]
	}
	d.keyLabels = make(map[string]string)
	for _, label := range opts.StringSlice(flagKeyLabel) {
		split := strings.SplitN(label, "=", 2)
		if len(split) != 2 {
			return fmt.Errorf("key label %v is not in key=value format", label)
		}
		d.keyLabels[split[0]] = split[1]
	}
	return nil
}
