package minecraftspark

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

const nestedJarMaximumBytes = 64 * 1024 * 1024

// RequiredJavaMajor 返回一个 Jar 中基础 class 文件所要求的最高 Java 版本。
// Fabric Jar-in-Jar 包装（如 fabric-api 顶层无任何 class 文件）会下钻 META-INF/jars 内嵌 Jar 取最高要求。
func RequiredJavaMajor(jarPath string) (int, error) {
	archive, err := zip.OpenReader(jarPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = archive.Close() }()

	maximum, err := scanClassFileMajors(&archive.Reader)
	if err != nil {
		return 0, err
	}
	if maximum == 0 {
		maximum, err = scanNestedJarMajors(&archive.Reader)
		if err != nil {
			return 0, err
		}
	}
	if maximum == 0 {
		return 0, errors.New("jar does not contain base class files")
	}
	return maximum, nil
}

// scanClassFileMajors 返回已打开 Jar 内基础 class 文件的最高 Java 要求;不存在时返回 0。
func scanClassFileMajors(archive *zip.Reader) (int, error) {
	maximum := 0
	for _, file := range archive.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if !strings.HasSuffix(strings.ToLower(name), ".class") || strings.HasPrefix(strings.ToUpper(name), "META-INF/VERSIONS/") {
			continue
		}
		reader, openErr := file.Open()
		if openErr != nil {
			return 0, openErr
		}
		header := make([]byte, 8)
		_, readErr := io.ReadFull(reader, header)
		_ = reader.Close()
		if readErr != nil {
			return 0, readErr
		}
		if binary.BigEndian.Uint32(header[:4]) != 0xCAFEBABE {
			return 0, fmt.Errorf("invalid class file magic: %s", name)
		}
		classMajor := int(binary.BigEndian.Uint16(header[6:8]))
		if classMajor < 45 {
			return 0, fmt.Errorf("unsupported class file major %d: %s", classMajor, name)
		}
		javaMajor := classMajor - 44
		if javaMajor > maximum {
			maximum = javaMajor
		}
	}
	return maximum, nil
}

// scanNestedJarMajors 返回 META-INF/jars 下 Fabric 内嵌 Jar 中的最高 Java 要求。
func scanNestedJarMajors(archive *zip.Reader) (int, error) {
	maximum := 0
	for _, file := range archive.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if !strings.HasPrefix(strings.ToUpper(name), "META-INF/JARS/") || !strings.HasSuffix(strings.ToLower(name), ".jar") {
			continue
		}
		reader, openErr := file.Open()
		if openErr != nil {
			return 0, openErr
		}
		payload, readErr := io.ReadAll(io.LimitReader(reader, nestedJarMaximumBytes+1))
		_ = reader.Close()
		if readErr != nil {
			return 0, readErr
		}
		if len(payload) > nestedJarMaximumBytes {
			return 0, fmt.Errorf("nested jar exceeds size limit: %s", name)
		}
		nested, nestedErr := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
		if nestedErr != nil {
			return 0, fmt.Errorf("open nested jar %s: %w", name, nestedErr)
		}
		nestedMaximum, scanErr := scanClassFileMajors(nested)
		if scanErr != nil {
			return 0, fmt.Errorf("scan nested jar %s: %w", name, scanErr)
		}
		if nestedMaximum > maximum {
			maximum = nestedMaximum
		}
	}
	return maximum, nil
}
