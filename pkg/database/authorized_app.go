// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package database

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/jinzhu/gorm"
)

type APIUserType int

const (
	apiKeyBytes = 64 // 64 bytes is 86 chararacters in non-padded base64.

	APIUserTypeDevice APIUserType = 0
	APIUserTypeAdmin  APIUserType = 1
)

// AuthorizedApp represents an application that is authorized to verify
// verification codes and perform token exchanges.
// This is controlled via a generated API key.
//
// Admin Keys are able to issue diagnosis keys and are not able to perticipate
// the verification protocol.
type AuthorizedApp struct {
	gorm.Model
	// AuthorizedApps belong to exactly one realm.
	RealmID    uint        `gorm:"unique_index:realm_apikey_name"`
	Realm      *Realm      // for loading the owning realm.
	Name       string      `gorm:"type:varchar(100);unique_index:realm_apikey_name"`
	APIKey     string      `gorm:"type:varchar(100);unique_index"`
	APIKeyType APIUserType `gorm:"default:0"`
}

func (a *AuthorizedApp) IsAdminType() bool {
	return a.APIKeyType == APIUserTypeAdmin
}

func (a *AuthorizedApp) IsDeviceType() bool {
	return a.APIKeyType == APIUserTypeDevice
}

// GetRealm does a lazy load read of the realm associated with this
// authorized app.
func (a *AuthorizedApp) GetRealm(db *Database) (*Realm, error) {
	if a.Realm != nil {
		return a.Realm, nil
	}
	var realm Realm
	if err := db.db.Model(a).Related(&realm).Error; err != nil {
		return nil, err
	}
	a.Realm = &realm
	return a.Realm, nil
}

// TODO(mikehelmick): Implement revoke API key functionality.

// TableName definition for the authorized apps relation.
func (AuthorizedApp) TableName() string {
	return "authorized_apps"
}

// ListAuthorizedApps retrieves all of the configured authorized apps.
// Done without pagination, as the expected number of authorized apps
// is low signal digits.
func (db *Database) ListAuthorizedApps(includeDeleted bool) ([]*AuthorizedApp, error) {
	var apps []*AuthorizedApp

	scope := db.db
	if includeDeleted {
		scope = db.db.Unscoped()
	}
	if err := scope.Preload("Realm").Order("name ASC").Find(&apps).Error; err != nil {
		return nil, fmt.Errorf("query authorized apps: %w", err)
	}
	return apps, nil
}

// CreateAuthorizedApp generates a new APIKey and assigns it to the specified
// name.
func (db *Database) CreateAuthorizedApp(realmID uint, name string, apiUserType APIUserType) (*AuthorizedApp, error) {
	if !(apiUserType == APIUserTypeAdmin || apiUserType == APIUserTypeDevice) {
		return nil, fmt.Errorf("invalid API Key user type requested: %v", apiUserType)
	}

	buffer := make([]byte, apiKeyBytes)
	_, err := rand.Read(buffer)
	if err != nil {
		return nil, fmt.Errorf("rand.Read: %v", err)
	}

	app := AuthorizedApp{
		Name:       name,
		APIKey:     base64.RawStdEncoding.EncodeToString(buffer),
		APIKeyType: apiUserType,
		RealmID:    realmID,
	}
	if err := db.db.Create(&app).Error; err != nil {
		return nil, fmt.Errorf("unable to save authorized app: %w", err)
	}
	return &app, nil
}

// FindAuthorizedAppByAPIKey located an authorized app based on API key. If no
// app exists for the given API key, it returns nil.
func (db *Database) FindAuthorizedAppByAPIKey(apiKey string) (*AuthorizedApp, error) {
	var app AuthorizedApp
	if err := db.db.Preload("Realm").Where("api_key = ?", apiKey).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &app, nil
}
