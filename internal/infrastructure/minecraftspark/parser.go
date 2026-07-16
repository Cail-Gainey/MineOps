package minecraftspark

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const (
	tpsHeading         = "TPS from last 5s, 10s, 1m, 5m, 15m:"
	legacyMSPTHeading  = "Tick durations (min/med/95%ile/max ms) from last 10s:"
	currentMSPTHeading = "Tick durations (min/med/95%ile/max ms) from last 10s, 1m:"
	sparkPrefix        = "[⚡]"
)

// ParseTPSResponse parses the locked Spark console TPS and MSPT response grammar.
func ParseTPSResponse(key AdapterKey, raw string) (Report, error) {
	if !SupportsPluginVersion(key.PluginVersion) ||
		key.CollectionMethod != CollectionRCONText ||
		key.ParserVersion != ParserVersion ||
		key.SourceSchemaHash != SourceSchemaHash ||
		!supportsBaselineTextParser(key.Platform) {
		return Report{}, fmt.Errorf("%w: adapter key %+v", ErrUnsupported, key)
	}

	lines := make([]string, 0, 16)
	sparkLines := make([]string, 0, 16)
	for _, line := range strings.Split(stripStyles(raw), "\n") {
		line = normalizeConsoleLine(line)
		if line != "" {
			lines = append(lines, line)
			if strings.HasPrefix(line, sparkPrefix) {
				sparkLines = append(sparkLines, strings.TrimSpace(strings.TrimPrefix(line, sparkPrefix)))
			}
		}
	}
	if len(sparkLines) > 0 {
		lines = sparkLines
	} else {
		for index, line := range lines {
			lines[index] = strings.TrimSpace(strings.TrimPrefix(line, sparkPrefix))
		}
	}
	start := -1
	for index, line := range lines {
		if line == tpsHeading {
			start = index
		}
	}
	if start < 0 || start+1 >= len(lines) {
		return Report{}, fmt.Errorf("%w: expected TPS heading and values", ErrUnsupported)
	}
	tpsValues, tpsCapped, err := parseTPSValues(lines[start+1], 5)
	if err != nil {
		return Report{}, fmt.Errorf("%w: TPS values: %v", ErrUnsupported, err)
	}
	report := Report{
		Adapter: key,
		Snapshot: Snapshot{TPS: TPSWindows{
			Last5Seconds: tpsValues[0], Last5SecondsCapped: tpsCapped[0],
			Last10Seconds: tpsValues[1], Last10SecondsCapped: tpsCapped[1],
			Last1Minute: tpsValues[2], Last1MinuteCapped: tpsCapped[2],
			Last5Minutes: tpsValues[3], Last5MinutesCapped: tpsCapped[3],
			Last15Minutes: tpsValues[4], Last15MinutesCapped: tpsCapped[4],
		}},
	}
	for index := start + 2; index+1 < len(lines); index++ {
		if lines[index] != legacyMSPTHeading && lines[index] != currentMSPTHeading {
			continue
		}
		msptWindow := strings.TrimSpace(strings.SplitN(lines[index+1], ";", 2)[0])
		msptValues, parseErr := parseFixedValues(msptWindow, "/", 4)
		if parseErr != nil {
			return Report{}, fmt.Errorf("%w: MSPT values: %v", ErrUnsupported, parseErr)
		}
		report.Snapshot.MSPT = &MSPTDistribution{
			Minimum: msptValues[0],
			Median:  msptValues[1],
			P95:     msptValues[2],
			Maximum: msptValues[3],
		}
		break
	}
	return report, nil
}

func supportsBaselineTextParser(platform PlatformFamily) bool {
	switch platform {
	case PlatformPaper, PlatformSpigot, PlatformFabric, PlatformForge, PlatformNeoForge, PlatformSponge:
		return true
	default:
		return false
	}
}

func normalizeConsoleLine(raw string) string {
	line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
	if marker := strings.LastIndex(line, "]: "); marker >= 0 {
		line = strings.TrimSpace(line[marker+3:])
	}
	return line
}

func parseTPSValues(raw string, expected int) ([]float64, []bool, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != expected {
		return nil, nil, fmt.Errorf("expected %d values", expected)
	}
	values := make([]float64, expected)
	capped := make([]bool, expected)
	for index, part := range parts {
		value := strings.TrimSpace(part)
		capped[index] = strings.HasPrefix(value, "*")
		value = strings.TrimPrefix(value, "*")
		if value == "" || strings.Contains(value, "*") {
			return nil, nil, fmt.Errorf("invalid numeric token %q", part)
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("parse %q: %w", part, err)
		}
		values[index] = parsed
	}
	return values, capped, nil
}

func parseFixedValues(raw, separator string, expected int) ([]float64, error) {
	parts := strings.Split(raw, separator)
	if len(parts) != expected {
		return nil, fmt.Errorf("expected %d values", expected)
	}
	values := make([]float64, expected)
	for index, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, fmt.Errorf("invalid numeric token %q", part)
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", part, err)
		}
		values[index] = parsed
	}
	return values, nil
}

func stripStyles(raw string) string {
	var cleaned strings.Builder
	cleaned.Grow(len(raw))
	for index := 0; index < len(raw); {
		if raw[index] == 0x1b && index+1 < len(raw) && raw[index+1] == '[' {
			index += 2
			for index < len(raw) {
				character := raw[index]
				index++
				if character >= 0x40 && character <= 0x7e {
					break
				}
			}
			continue
		}
		if raw[index] == 0xc2 && index+2 < len(raw) && raw[index+1] == 0xa7 {
			index += 3
			continue
		}
		if raw[index] == '<' {
			closing := strings.IndexByte(raw[index:], '>')
			if closing > 0 {
				tag := raw[index+1 : index+closing]
				validTag := tag != ""
				for _, character := range tag {
					if !unicode.IsLetter(character) && !unicode.IsDigit(character) && !strings.ContainsRune("/_:-#", character) {
						validTag = false
						break
					}
				}
				if validTag {
					index += closing + 1
					continue
				}
			}
		}
		cleaned.WriteByte(raw[index])
		index++
	}
	return cleaned.String()
}
