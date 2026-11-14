package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type iniTarget struct{ section, key, value string }

var (
	sectionLine = regexp.MustCompile(`^\[([^\]]+)\][ \t]*(?:[;#].*)?$`)
	valueLine   = regexp.MustCompile(`^([^=]*=[ \t]*)([^;#]*?)([ \t]*(?:[;#].*)?)$`)
)

// editINI preserves every byte outside the target values and inserted keys.
func editINI(data []byte, address, port string) ([]byte, bool, error) {
	targets := []iniTarget{
		{"General", "cur_remote_protocol_index", "3"},
		{"ServiceSettings", "global_app_mode", "1"},
		{"GLINTSettings", "start_glint_session", "0"},
	}
	if address != "" {
		targets = append(targets,
			iniTarget{"RemoteViewerConnect", "port", port},
			iniTarget{"RemoteViewerConnect", "broker_addresses_list", address})
	}
	if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
		return nil, false, errors.New("INI: поддерживается UTF-8 без NUL; файл не изменён")
	}
	bom := ""
	src := string(data)
	if strings.HasPrefix(src, "\xef\xbb\xbf") {
		bom, src = "\xef\xbb\xbf", src[3:]
	}
	if strings.Contains(strings.ReplaceAll(src, "\r\n", ""), "\r") {
		return nil, false, errors.New("INI: неподдерживаемые окончания строк")
	}
	eol := "\n"
	if strings.Contains(src, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.SplitAfter(src, "\n")
	sections, keys := map[string]bool{}, map[string]bool{}
	ends, found := map[string]int{}, map[string]bool{}
	section, ready := "", true
	for i, line := range lines {
		body := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		t := strings.TrimSpace(body)
		if strings.HasPrefix(t, "[") {
			var err error
			section, err = parseSection(t, i+1, sections, targets)
			if err != nil {
				return nil, false, err
			}
		} else if t != "" && !strings.HasPrefix(t, ";") && !strings.HasPrefix(t, "#") {
			edited, matches, err := editKey(line, body, section, i+1, keys, found, targets)
			if err != nil {
				return nil, false, err
			}
			lines[i], ready = edited, ready && matches
		}
		ends[section] = i
	}
	for _, target := range targets {
		if found[target.section+"\x00"+target.key] {
			continue
		}
		ready = false
		addition := target.key + "=" + target.value + eol
		if sections[strings.ToLower(target.section)] {
			i := ends[target.section]
			if lines[i] != "" && !strings.HasSuffix(lines[i], "\n") {
				lines[i] += eol
			}
			lines[i] += addition
		} else {
			i := len(lines) - 1
			if lines[i] != "" && !strings.HasSuffix(lines[i], "\n") {
				lines[i] += eol
			}
			lines[i] += "[" + target.section + "]" + eol + addition
			sections[strings.ToLower(target.section)] = true
			ends[target.section] = i
		}
	}

	return []byte(bom + strings.Join(lines, "")), ready, nil
}

func parseSection(text string, lineNumber int, sections map[string]bool, targets []iniTarget) (string, error) {
	m := sectionLine.FindStringSubmatch(text)
	if m == nil {
		return "", fmt.Errorf("INI: некорректная секция, строка %d", lineNumber)
	}
	section := m[1]
	if section == "" || section != strings.TrimSpace(section) {
		return "", fmt.Errorf("INI: неоднозначное имя секции, строка %d", lineNumber)
	}
	if sections[strings.ToLower(section)] {
		return "", fmt.Errorf("INI: дубликат секции [%s], строка %d", section, lineNumber)
	}
	sections[strings.ToLower(section)] = true
	for _, target := range targets {
		if strings.EqualFold(section, target.section) && section != target.section {
			return "", fmt.Errorf("INI: неоднозначный регистр секции, строка %d", lineNumber)
		}
	}

	return section, nil
}

func editKey(line, body, section string, lineNumber int, keys, found map[string]bool,
	targets []iniTarget,
) (string, bool, error) {
	ready := true
	key, _, ok := strings.Cut(body, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return "", false, fmt.Errorf("INI: неоднозначная строка %d", lineNumber)
	}
	id := strings.ToLower(section + "\x00" + key)
	if keys[id] {
		return "", false, fmt.Errorf("INI: дубликат [%s] %s, строка %d", section, key, lineNumber)
	}
	keys[id] = true
	for _, target := range targets {
		if section == target.section && strings.EqualFold(key, target.key) && key != target.key {
			return "", false, fmt.Errorf("INI: неоднозначный регистр ключа, строка %d", lineNumber)
		}
		if section == target.section && key == target.key {
			m := valueLine.FindStringSubmatch(body)
			if m == nil {
				return "", false, fmt.Errorf("INI: неоднозначное значение, строка %d", lineNumber)
			}
			found[section+"\x00"+key] = true
			ready = strings.TrimSpace(m[2]) == target.value
			line = m[1] + target.value + m[3] + line[len(body):]
		}
	}

	return line, ready, nil
}
