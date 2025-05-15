package metal

import (
	"fmt"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

// An Image describes an image which could be used for provisioning.
type Image struct {
	Base
	URL      string                    `rethinkdb:"url"`
	Features map[ImageFeatureType]bool `rethinkdb:"features"`
	// OS is parsed from id and is the first part, specifies operating system derivate, internal usage only
	OS string `rethinkdb:"os"`
	// Version is parsed from id and is the second part, specifies operating system version, internal usage only
	Version string `rethinkdb:"version"`
	// ExpirationDate defines the time in the future, when this image is not considered for machine allocations anymore
	ExpirationDate time.Time `rethinkdb:"expirationDate"`
	// Classification defines the state of a version (preview, supported, deprecated)
	// only informational, no action depending on the classification done
	Classification VersionClassification `rethinkdb:"classification"`
}

// DefaultImageExpiration if not specified images will last for about 3 month
var DefaultImageExpiration = time.Hour * 24 * 90

// VersionClassification is the logical state of a version
type VersionClassification string

const (
	// ClassificationPreview indicates that a version has recently been added and not promoted to "Supported" yet.
	// ClassificationPreview versions will not be considered for automatic OperatingSystem patch version updates.
	ClassificationPreview VersionClassification = "preview"
	// ClassificationSupported indicates that a patch version is the default version for the particular minor version.
	// There is always exactly one supported OperatingSystem patch version for every still maintained OperatingSystem minor version.
	// Supported versions are eligible for the automated OperatingSystem patch version update machines.
	ClassificationSupported VersionClassification = "supported"
	// ClassificationDeprecated indicates that a patch version should not be used anymore, should be updated to a new version
	// and will eventually expire.
	// Every version that is neither in preview nor supported is deprecated.
	// All patch versions of not supported minor versions are deprecated.
	ClassificationDeprecated VersionClassification = "deprecated"
)

// VersionClassificationFrom create a VersionClassification from api type
func VersionClassificationFrom(classification apiv2.ImageClassification) (VersionClassification, error) {
	switch classification {
	case apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW, apiv2.ImageClassification_IMAGE_CLASSIFICATION_UNSPECIFIED:
		return ClassificationPreview, nil
	case apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED:
		return ClassificationSupported, nil
	case apiv2.ImageClassification_IMAGE_CLASSIFICATION_DEPRECATED:
		return ClassificationDeprecated, nil
	default:
		return "", fmt.Errorf("given versionclassification is not valid:%s", classification.String())
	}
}

// ImageFeatureType specifies the features of a images
type ImageFeatureType string

const (
	// ImageFeatureFirewall from this image only a firewall can created
	ImageFeatureFirewall ImageFeatureType = "firewall"
	// ImageFeatureMachine from this image only a machine can created
	ImageFeatureMachine ImageFeatureType = "machine"
)

func ImageFeaturesFrom(features []apiv2.ImageFeature) (map[ImageFeatureType]bool, error) {
	var result = make(map[ImageFeatureType]bool)
	for _, feature := range features {
		switch feature {
		case apiv2.ImageFeature_IMAGE_FEATURE_MACHINE:
			result[ImageFeatureMachine] = true
		case apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL:
			result[ImageFeatureFirewall] = true
		case apiv2.ImageFeature_IMAGE_FEATURE_UNSPECIFIED:
			return nil, fmt.Errorf("unspecified imagefeature:%s", feature.String())
		default:
			return nil, fmt.Errorf("unknown imagefeature:%s", feature.String())
		}
	}

	return result, nil
}
