// Package gradle читает и переписывает файлы Gradle-проекта
// (gradle.properties, build.gradle / .kts, catalog, wrapper) целевыми
// regex и построчными преобразованиями — не полным парсером DSL.
package gradle

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Property — пара ключ=значение из gradle.properties.
type Property struct {
	Key   string // имя свойства, например "orbitCoreVersion"
	Value string // значение, например "2.4.1"
}

// ParseProperties читает файл gradle.properties.
//
// Возвращает:
//   - map[ключ]значение — при дубликатах побеждает последнее значение;
//   - порядок первых вхождений ключей (чтобы отчёт был стабильным);
//   - ошибку чтения/сканирования.
//
// Пропускает пустые строки и комментарии (# и !).
func ParseProperties(path string) (map[string]string, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	values := make(map[string]string)
	var order []string
	seen := make(map[string]bool)

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		key, value, ok := splitProp(line)
		if !ok {
			continue
		}
		values[key] = value
		if !seen[key] {
			order = append(order, key)
			seen[key] = true
		}
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	return values, order, nil
}

// splitProp разбирает строку "key=value" или "key:value".
// Возвращает ok=false, если разделителя нет или ключ пустой.
func splitProp(line string) (string, string, bool) {
	idx := strings.IndexAny(line, "=:")
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

// IsVersionProperty отвечает, похоже ли свойство на версию зависимости.
//
// Отсекает:
//   - пустые ключ/значение;
//   - настройки Gradle (org.gradle.*);
//   - boolean true/false;
//   - строки без цифр или с «левыми» символами (пробелы и т.п.).
//
// Это эвристика, не строгая схема Maven.
func IsVersionProperty(key, value string) bool {
	if key == "" || value == "" {
		return false
	}
	if strings.HasPrefix(key, "org.gradle.") {
		return false
	}
	switch strings.ToLower(value) {
	case "true", "false":
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '+' {
			continue
		}
		return false
	}
	hasDigit := false
	for _, r := range value {
		if r >= '0' && r <= '9' {
			hasDigit = true
			break
		}
	}
	return hasDigit
}

// UpdatePropertiesValues обновляет значения ключей в gradle.properties построчно.
//
// Не переписывает файл целиком «с нуля»: сохраняет комментарии, пустые строки
// и ключи, которых нет в updates. Если updates пуст или нечего менять — no-op.
func UpdatePropertiesValues(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		key, _, ok := splitProp(trimmed)
		if !ok {
			continue
		}
		newVal, want := updates[key]
		if !want {
			continue
		}
		sep := "="
		if idx := strings.IndexAny(line, "=:"); idx >= 0 {
			sep = string(line[idx])
		}
		lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = fmt.Sprintf("%s%s%s%s", lead, key, sep, newVal)
		changed = true
	}
	if !changed {
		return nil
	}
	out := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(out), 0o644)
}
