/*
 * Copyright (c) 2019 Andrew Ayer
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 * Except as contained in this notice, the name(s) of the above copyright
 * holders shall not be used in advertising or otherwise to promote the
 * sale, use or other dealings in this Software without prior written
 * authorization.
 */

package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"io/ioutil"
	"strings"
)

func loadConfigFile(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	config := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 {
			config[fields[0]] = fields[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Error reading %s: %s", filename, err)
	}
	return config, nil
}

func loadUsersFile(filename string) (map[string]UserConfig, error) {
	params, err := loadConfigFile(filename)
	if err != nil {
		return nil, err
	}
	users := make(map[string]UserConfig)
	for userJID, userSpec := range params {
		fields := strings.SplitN(userSpec, ":", 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("User %s in %s has malformed configuration (should look like provider:phonenumber)", userJID, filename)
		}
		users[userJID] = UserConfig{
			Provider: fields[0],
			PhoneNumber: fields[1],
		}
	}
	return users, nil
}

func loadProviderConfigFile(filename string) (ProviderConfig, error) {
	params, err := loadConfigFile(filename)
	if err != nil {
		return ProviderConfig{}, err
	}

	typeParam, exists := params["type"]
	if !exists {
		return ProviderConfig{}, fmt.Errorf("%s lacks type parameter", filename)
	}
	delete(params, "type")
	return ProviderConfig{
		Type: typeParam,
		Params: params,
	}, nil
}

func loadProvidersDirectory(dirpath string) (map[string]ProviderConfig, error) {
	providerFiles, err := ioutil.ReadDir(dirpath)
	if err != nil {
		return nil, err
	}
	providers := make(map[string]ProviderConfig)
	for _, providerFile := range providerFiles {
		providerFileName := providerFile.Name()
		if strings.HasPrefix(providerFileName, ".") {
			continue
		}
		providerConfig, err := loadProviderConfigFile(filepath.Join(dirpath, providerFileName))
		if err != nil {
			return nil, err
		}
		providers[providerFileName] = providerConfig
	}
	return providers, nil
}

func FromDirectory(dirpath string) (*Config, error) {
	config := new(Config)

	params, err := loadConfigFile(filepath.Join(dirpath, "config"))
	if err != nil {
		return nil, err
	}
	config.XMPPServer = params["xmpp_server"]
	config.XMPPDomain = params["xmpp_domain"]
	config.XMPPSecret = params["xmpp_secret"]
	config.PublicURL = params["public_url"]
	config.Users, err = loadUsersFile(filepath.Join(dirpath, "users"))
	if err != nil {
		return nil, err
	}
	config.Providers, err = loadProvidersDirectory(filepath.Join(dirpath, "providers"))
	if err != nil {
		return nil, err
	}
	config.Rosters, err = loadConfigFile(filepath.Join(dirpath, "rosters"))
	if err != nil {
		return nil, err
	}
	return config, nil
}
