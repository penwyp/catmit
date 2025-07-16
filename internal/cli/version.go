package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionRegex = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([^+]+))?(?:\+(.+))?$`)

// ParseVersion 解析语义化版本字符串
func ParseVersion(versionStr string) (Version, error) {
	if versionStr == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	matches := versionRegex.FindStringSubmatch(versionStr)
	if matches == nil {
		return Version{}, fmt.Errorf("invalid version format: %s", versionStr)
	}

	var v Version

	// Major version (required)
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %s", matches[1])
	}
	v.Major = major

	// Minor version (optional, defaults to 0)
	if matches[2] != "" {
		minor, err := strconv.Atoi(matches[2])
		if err != nil {
			return Version{}, fmt.Errorf("invalid minor version: %s", matches[2])
		}
		v.Minor = minor
	}

	// Patch version (optional, defaults to 0)
	if matches[3] != "" {
		patch, err := strconv.Atoi(matches[3])
		if err != nil {
			return Version{}, fmt.Errorf("invalid patch version: %s", matches[3])
		}
		v.Patch = patch
	}

	// Pre-release (optional)
	if matches[4] != "" {
		v.PreRelease = matches[4]
	}

	// Build metadata (optional)
	if matches[5] != "" {
		v.Build = matches[5]
	}

	return v, nil
}

// CompareVersions 比较两个版本字符串
// 返回: -1 (v1 < v2), 0 (v1 == v2), 1 (v1 > v2)
func CompareVersions(v1Str, v2Str string) int {
	v1, err1 := ParseVersion(v1Str)
	v2, err2 := ParseVersion(v2Str)

	// 如果解析失败，简单字符串比较
	if err1 != nil || err2 != nil {
		return strings.Compare(v1Str, v2Str)
	}

	// 比较主版本号
	if v1.Major != v2.Major {
		if v1.Major > v2.Major {
			return 1
		}
		return -1
	}

	// 比较次版本号
	if v1.Minor != v2.Minor {
		if v1.Minor > v2.Minor {
			return 1
		}
		return -1
	}

	// 比较修订版本号
	if v1.Patch != v2.Patch {
		if v1.Patch > v2.Patch {
			return 1
		}
		return -1
	}

	// 比较预发布版本
	// 没有预发布版本的版本高于有预发布版本的
	if v1.PreRelease == "" && v2.PreRelease != "" {
		return 1
	}
	if v1.PreRelease != "" && v2.PreRelease == "" {
		return -1
	}

	// 如果都有预发布版本，进行字符串比较
	if v1.PreRelease != "" && v2.PreRelease != "" {
		result := comparePreRelease(v1.PreRelease, v2.PreRelease)
		if result != 0 {
			return result
		}
	}

	// 构建元数据不影响版本比较
	return 0
}

// comparePreRelease 比较预发布版本
func comparePreRelease(pre1, pre2 string) int {
	// 简化的预发布版本比较
	// 实际semver规范更复杂，这里做简化处理
	parts1 := strings.Split(pre1, ".")
	parts2 := strings.Split(pre2, ".")

	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		// 尝试数字比较
		num1, err1 := strconv.Atoi(parts1[i])
		num2, err2 := strconv.Atoi(parts2[i])

		if err1 == nil && err2 == nil {
			// 都是数字
			if num1 != num2 {
				if num1 > num2 {
					return 1
				}
				return -1
			}
		} else {
			// 字符串比较
			cmp := strings.Compare(parts1[i], parts2[i])
			if cmp != 0 {
				return cmp
			}
		}
	}

	// 更多部分的版本号更高
	if len(parts1) > len(parts2) {
		return 1
	}
	if len(parts1) < len(parts2) {
		return -1
	}

	return 0
}

// CheckMinVersion 检查当前版本是否满足最低版本要求
func CheckMinVersion(current, minimum string) (bool, error) {
	// 解析版本以验证格式
	_, err := ParseVersion(current)
	if err != nil {
		return false, fmt.Errorf("invalid current version: %w", err)
	}

	_, err = ParseVersion(minimum)
	if err != nil {
		return false, fmt.Errorf("invalid minimum version: %w", err)
	}

	// 使用比较函数
	result := CompareVersions(current, minimum)
	return result >= 0, nil
}