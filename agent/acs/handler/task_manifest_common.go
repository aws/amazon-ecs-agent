// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package handler

import (
	"sync"
)

// manifestMessageIDAccessor struct implements ManifestMessageIDAccessor interface defined in ecs-agent module.
type manifestMessageIDAccessor struct {
	messageID string
	lock      sync.RWMutex
}

// NewManifestMessageIDAccessor creates a new manifestMessageIDAccessor.
func NewManifestMessageIDAccessor() *manifestMessageIDAccessor {
	return &manifestMessageIDAccessor{}
}

func (mmiAccessor *manifestMessageIDAccessor) GetMessageID() string {
	mmiAccessor.lock.RLock()
	defer mmiAccessor.lock.RUnlock()
	return mmiAccessor.messageID
}

func (mmiAccessor *manifestMessageIDAccessor) SetMessageID(messageId string) error {
	mmiAccessor.lock.Lock()
	defer mmiAccessor.lock.Unlock()
	mmiAccessor.messageID = messageId
	return nil
}
