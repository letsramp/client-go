/*
 * Copyright Skyramp Authors 2024
 */
package types

import "fmt"

func (s *TestStep) GetResponseValue(jsonPath string) string {
	if s.RequestName != "" {
		if jsonPath != "" {
			return fmt.Sprintf("requests.%s.res.%s", s.RequestName, jsonPath)
		}
		return fmt.Sprintf("requests.%s.res", s.RequestName)
	} else {
		return ""
	}
}

func (s *TestStep) GetCookieValue(jsonPath string) string {
	if s.RequestName != "" {
		if jsonPath != "" {
			return fmt.Sprintf("requests.%s.cookies.%s", s.RequestName, jsonPath)
		}
		return fmt.Sprintf("requests.%s.cookies", s.RequestName)
	} else {
		return ""
	}
}

func (s *TestStep) GetResponseCode() string {
	if s.RequestName != "" {
		return fmt.Sprintf("requests.%s.code", s.RequestName)
	} else {
		return ""
	}
}

func (s *TestStep) SetValue(overrideMap map[string]string) {
	s.Override = overrideMap
}

func (s *TestStep) GetExportValue(exportMap map[string]string) {
	s.Export = exportMap
}
