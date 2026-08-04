// mixkey.go - Katzenpost server mix key store.
// Copyright (C) 2017  Yawning Angel.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package mixkeys

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/op/go-logging.v1"

	"github.com/katzenpost/hpqc/kem"
	"github.com/katzenpost/hpqc/nike"

	"github.com/katzenpost/katzenpost/core/epochtime"
	"github.com/katzenpost/katzenpost/core/sphinx/geo"
	"github.com/katzenpost/katzenpost/server/internal/constants"
	"github.com/katzenpost/katzenpost/server/internal/glue"
	"github.com/katzenpost/katzenpost/server/internal/mixkey"
)

type mixKeys struct {
	sync.Mutex

	geo     *geo.Geometry
	glue    glue.Glue
	log     *logging.Logger
	dataDir string

	keys map[uint64]*mixkey.MixKey

	nike nike.Scheme
	kem  kem.Scheme
}

func (m *mixKeys) init() error {
	// Generate/load the initial set of keys.
	//
	// TODO: In theory this should also try to load the previous epoch's key
	// if the current time is in the clock skew grace period.  But it may not
	// matter much in practice.
	epoch, _, _ := epochtime.Now()
	if _, err := m.Generate(epoch); err != nil {
		return err
	}

	return nil
}

func (m *mixKeys) Generate(baseEpoch uint64) (bool, error) {
	didGenerate := false

	m.Lock()
	defer m.Unlock()
	m.log.Debugf("Generate: baseEpoch=%d, dataDir=%q, numKeys=%d", baseEpoch, m.dataDir, constants.NumMixKeys)
	for e := baseEpoch; e < baseEpoch+constants.NumMixKeys; e++ {
		if _, ok := m.keys[e]; ok {
			continue
		}

		keyPath := filepath.Join(m.dataDir, fmt.Sprintf("mixkey-%d.key", e))
		if m.dataDir != "" {
			m.log.Debugf("Generate: checking disk for %s", keyPath)
			if loaded, loadErr := m.loadKeyFromDisk(keyPath, e); loadErr == nil && loaded != nil {
				m.log.Debugf("Generate: loaded key for epoch %d from disk", e)
				m.keys[e] = loaded
				didGenerate = true
				continue
			}
		}

		didGenerate = true
		k, err := mixkey.New(e, m.geo)
		if err != nil {
			for ee := baseEpoch; ee < baseEpoch+constants.NumMixKeys; ee++ {
				if kk, ok := m.keys[ee]; ok {
					kk.Deref()
					delete(m.keys, ee)
				}
			}
			return false, err
		}
		k.SetUnlinkIfExpired(true)
		m.keys[e] = k

		if m.dataDir != "" {
			m.log.Debugf("Generate: saving key for epoch %d to %s", e, keyPath)
			if saveErr := m.saveKeyToDisk(keyPath, k); saveErr != nil {
				m.log.Warningf("Failed to persist mix key for epoch %d: %v", e, saveErr)
			} else {
				m.log.Debugf("Generate: saved key for epoch %d", e)
			}
		}
	}

	return didGenerate, nil
}

func (m *mixKeys) Prune() bool {
	epoch, _, _ := epochtime.Now()
	didPrune := false

	m.Lock()
	defer m.Unlock()

	for idx, v := range m.keys {
		if idx < epoch-1 {
			m.log.Debugf("Purging expired key for epoch: %v", idx)
			v.Deref()
			delete(m.keys, idx)
			didPrune = true
		}
	}

	return didPrune
}

func (m *mixKeys) Get(epoch uint64) ([]byte, bool) {
	m.Lock()
	defer m.Unlock()

	if k, ok := m.keys[epoch]; ok {
		return k.PublicBytes(), true
	}
	return nil, false
}

func (m *mixKeys) Shadow(dst map[uint64]*mixkey.MixKey) {
	m.Lock()
	defer m.Unlock()

	// Purge the keys no longer listed from dst.
	for k, v := range dst {
		if _, ok := m.keys[k]; !ok {
			v.Deref()
			delete(dst, k)
		}
	}

	// Add newly listed keys to dst and bump up the refcount.
	for k, v := range m.keys {
		if _, ok := dst[k]; !ok {
			v.Ref()
			dst[k] = v
		}
	}
}

func (m *mixKeys) Halt() {
	m.Lock()
	defer m.Unlock()

	for k, v := range m.keys {
		v.Deref()
		delete(m.keys, k)
	}
}

// saveKeyToDisk persists a mix key to <dataDir>/mixkey-<epoch>.key.
// Format: 1-byte type (0=NIKE,1=KEM) + public key bytes + private key bytes.
// NIKE: type(1) + pub(32) + priv(32) = 65 bytes.
// KEM: type(1) + 2-byte pubLen + pub(pubLen) + priv = variable.
func (m *mixKeys) saveKeyToDisk(keyPath string, k *mixkey.MixKey) error {
	var raw []byte
	nikeScheme, kemScheme := m.geo.Scheme()
	if nikeScheme != nil {
		pub := k.PublicBytes()
		priv := k.PrivateKey().(nike.PrivateKey).Bytes()
		raw = make([]byte, 1+len(pub)+len(priv))
		raw[0] = 0
		copy(raw[1:], pub)
		copy(raw[1+len(pub):], priv)
	} else if kemScheme != nil {
		pub := k.PublicBytes()
		priv, err := k.PrivateKey().(kem.PrivateKey).MarshalBinary()
		if err != nil {
			return err
		}
		raw = make([]byte, 1+2+len(pub)+len(priv))
		raw[0] = 1
		raw[1] = byte(len(pub) >> 8)
		raw[2] = byte(len(pub))
		copy(raw[3:], pub)
		copy(raw[3+len(pub):], priv)
	} else {
		return fmt.Errorf("no NIKE or KEM scheme available")
	}
	return os.WriteFile(keyPath, raw, 0600)
}

// loadKeyFromDisk attempts to load a mix key from <dataDir>/mixkey-<epoch>.key.
// Returns (nil, nil) if the file doesn't exist or can't be parsed.
func (m *mixKeys) loadKeyFromDisk(keyPath string, epoch uint64) (*mixkey.MixKey, error) {
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil
	}
	if len(raw) < 2 {
		return nil, fmt.Errorf("key file too short: %d bytes", len(raw))
	}
	keyType := raw[0]
	nikeScheme, kemScheme := m.geo.Scheme()
	switch keyType {
	case 0:
		if nikeScheme == nil {
			return nil, fmt.Errorf("saved NIKE key but no NIKE scheme")
		}
		if len(raw) < 65 {
			return nil, fmt.Errorf("NIKE key file too short: %d bytes", len(raw))
		}
		pubBytes := raw[1:33]
		privBytes := raw[33:65]
		pubKey, err := nikeScheme.UnmarshalBinaryPublicKey(pubBytes)
		if err != nil {
			return nil, err
		}
		privKey, err := nikeScheme.UnmarshalBinaryPrivateKey(privBytes)
		if err != nil {
			return nil, err
		}
		loaded, err := mixkey.Init(epoch, pubKey.(nike.PublicKey), privKey.(nike.PrivateKey), nil, nil)
		if err != nil {
			return nil, err
		}
		loaded.SetUnlinkIfExpired(true)
		return loaded, nil
	case 1:
		if kemScheme == nil {
			return nil, fmt.Errorf("saved KEM key but no KEM scheme")
		}
		if len(raw) < 4 {
			return nil, fmt.Errorf("KEM key file too short: %d bytes", len(raw))
		}
		pubLen := int(raw[1])<<8 | int(raw[2])
		if pubLen <= 0 || 3+pubLen >= len(raw) {
			return nil, fmt.Errorf("invalid KEM key file format")
		}
		pubBytes := raw[3 : 3+pubLen]
		privBytes := raw[3+pubLen:]
		pubKey, err := kemScheme.UnmarshalBinaryPublicKey(pubBytes)
		if err != nil {
			return nil, err
		}
		privKey, err := kemScheme.UnmarshalBinaryPrivateKey(privBytes)
		if err != nil {
			return nil, err
		}
		loaded, err := mixkey.Init(epoch, nil, nil, pubKey.(kem.PublicKey), privKey.(kem.PrivateKey))
		if err != nil {
			return nil, err
		}
		loaded.SetUnlinkIfExpired(true)
		return loaded, nil
	default:
		return nil, fmt.Errorf("unknown key type: %d", keyType)
	}
}

func NewMixKeys(glue glue.Glue, geo *geo.Geometry, dataDir string) (glue.MixKeys, error) {
	m := &mixKeys{
		geo:     geo,
		glue:    glue,
		log:     glue.LogBackend().GetLogger("mixkeys"),
		dataDir: dataDir,
		keys:    make(map[uint64]*mixkey.MixKey),
	}

	if err := m.init(); err != nil {
		return nil, err
	}

	return m, nil
}
