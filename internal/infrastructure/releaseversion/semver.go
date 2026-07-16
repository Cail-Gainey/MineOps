// Package releaseversion provides strict reusable SemVer parsing and ordering for release catalogs.
package releaseversion

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type semanticVersion struct {
	major      string
	minor      string
	patch      string
	prerelease []string
}

// Compare compares two SemVer values, accepts an optional leading v, and returns -1, 0, or 1.
func Compare(current, target string) (int, error) {
	left, err := parse(current)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeValidationInvalidArgument, "当前版本不是有效 SemVer", err)
	}
	right, err := parse(target)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeValidationInvalidArgument, "目标版本不是有效 SemVer", err)
	}
	for _, pair := range [][2]string{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if comparison := compareNumericIdentifier(pair[0], pair[1]); comparison != 0 {
			return comparison, nil
		}
	}
	return comparePrerelease(left.prerelease, right.prerelease), nil
}

// Normalize validates one SemVer value and removes its optional leading v.
func Normalize(value string) (string, error) {
	if _, err := parse(value); err != nil {
		return "", apperror.Wrap(apperror.CodeValidationInvalidArgument, "版本不是有效 SemVer", err)
	}
	return strings.TrimPrefix(strings.TrimSpace(value), "v"), nil
}

// IsPrerelease reports whether one valid SemVer value contains prerelease identifiers.
func IsPrerelease(value string) (bool, error) {
	parsed, err := parse(value)
	if err != nil {
		return false, apperror.Wrap(apperror.CodeValidationInvalidArgument, "版本不是有效 SemVer", err)
	}
	return len(parsed.prerelease) > 0, nil
}

func parse(value string) (semanticVersion, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	matches := semverPattern.FindStringSubmatch(value)
	if matches == nil {
		return semanticVersion{}, fmt.Errorf("%q 不符合 SemVer", value)
	}
	version := semanticVersion{major: matches[1], minor: matches[2], patch: matches[3]}
	if matches[4] != "" {
		version.prerelease = strings.Split(matches[4], ".")
	}
	return version, nil
}

func compareNumericIdentifier(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return strings.Compare(left, right)
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	for index := 0; index < len(left) && index < len(right); index++ {
		leftNumeric := numericIdentifier(left[index])
		rightNumeric := numericIdentifier(right[index])
		switch {
		case leftNumeric && rightNumeric:
			if comparison := compareNumericIdentifier(left[index], right[index]); comparison != 0 {
				return comparison
			}
		case leftNumeric:
			return -1
		case rightNumeric:
			return 1
		default:
			if comparison := strings.Compare(left[index], right[index]); comparison != 0 {
				return comparison
			}
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func numericIdentifier(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value != ""
}
