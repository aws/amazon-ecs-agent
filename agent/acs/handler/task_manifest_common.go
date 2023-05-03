package handler

import (
	"sync"
)

type manifestMessageIDAccessor struct {
	messageID string
	lock      sync.RWMutex
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
