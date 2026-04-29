package state

import (
	"sync"
	"time"

	"github.com/DANG-PH/game-service-go/internal/shared/messages"
)

// PlayerState là snapshot mới nhất của 1 player.
// Lưu đầy đủ field giống PlayerMove/PlayerSync để khi tick broadcast
// có thể tái tạo PlayerSync packet ngay, không cần lookup thêm.
//
// Memory: ~150 byte/player (do có nhiều string). 1000 player ≈ 150KB → ổn.
type PlayerState struct {
	UserID int32
	MapID  string

	// Tất cả field này khớp với PlayerSync để Encode() trả về đúng định dạng.
	X              float32
	Y              float32
	Trangthai      uint8
	Dir            int8
	Dau            string
	Than           string
	Chan           string
	TimeChoHienBay float32
	LechDauX       float32
	LechDauY       float32
	LechThanX      float32
	LechThanY      float32
	LechChanX      float32
	LechChanY      float32
	FrameVanBay    uint16
	DangMangVanBay bool
	TenVanBay      string
	Rong           float32
	Cao            float32
	Avatar         string

	UpdatedAt int64 // unix milli — server timestamp khi nhận update
	Dirty     bool  // true nếu thay đổi từ tick trước
}

// ToSync convert state thành PlayerSync để encode.
// Tách hàm riêng cho rõ ràng và test được.
func (p *PlayerState) ToSync() *messages.PlayerSync {
	return &messages.PlayerSync{
		UserID:         p.UserID,
		X:              p.X,
		Y:              p.Y,
		Trangthai:      p.Trangthai,
		Dir:            p.Dir,
		Dau:            p.Dau,
		Than:           p.Than,
		Chan:           p.Chan,
		TimeChoHienBay: p.TimeChoHienBay,
		LechDauX:       p.LechDauX,
		LechDauY:       p.LechDauY,
		LechThanX:      p.LechThanX,
		LechThanY:      p.LechThanY,
		LechChanX:      p.LechChanX,
		LechChanY:      p.LechChanY,
		FrameVanBay:    p.FrameVanBay,
		DangMangVanBay: p.DangMangVanBay,
		TenVanBay:      p.TenVanBay,
		Rong:           p.Rong,
		Cao:            p.Cao,
		Avatar:         p.Avatar,
	}
}

// MapState gom tất cả player trong 1 map. Mỗi map có lock riêng.
type MapState struct {
	MapID string

	mu      sync.RWMutex
	players map[int32]*PlayerState
}

func newMapState(mapID string) *MapState {
	return &MapState{
		MapID:   mapID,
		players: make(map[int32]*PlayerState),
	}
}

// === Manager ===

type Manager struct {
	mu   sync.RWMutex
	maps map[string]*MapState
}

func NewManager() *Manager {
	return &Manager{maps: make(map[string]*MapState)}
}

func (m *Manager) GetOrCreateMap(mapID string) *MapState {
	// Double check locking
	m.mu.RLock()
	ms, ok := m.maps[mapID]
	m.mu.RUnlock()
	if ok {
		return ms
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if ms, ok := m.maps[mapID]; ok {
		return ms
	}
	ms = newMapState(mapID)
	m.maps[mapID] = ms
	return ms
}

func (m *Manager) GetMap(mapID string) (*MapState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ms, ok := m.maps[mapID]
	return ms, ok
}

func (m *Manager) AllMaps() []*MapState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*MapState, 0, len(m.maps))
	for _, ms := range m.maps {
		result = append(result, ms)
	}
	return result
}

// === MapState methods ===

// UpdateFromMove apply input từ client.
// Copy toàn bộ field từ PlayerMove vào PlayerState.
//
// TODO: anti-cheat validation ở đây (max speed, collision, ...).
func (ms *MapState) UpdateFromMove(userID int32, m *messages.PlayerMove) {
	now := time.Now().UnixMilli()

	ms.mu.Lock()
	defer ms.mu.Unlock()

	p, ok := ms.players[userID]
	if !ok {
		p = &PlayerState{
			UserID: userID,
			MapID:  ms.MapID,
		}
		ms.players[userID] = p
	}

	p.X = m.X
	p.Y = m.Y
	p.Trangthai = m.Trangthai
	p.Dir = m.Dir
	p.Dau = m.Dau
	p.Than = m.Than
	p.Chan = m.Chan
	p.TimeChoHienBay = m.TimeChoHienBay
	p.LechDauX = m.LechDauX
	p.LechDauY = m.LechDauY
	p.LechThanX = m.LechThanX
	p.LechThanY = m.LechThanY
	p.LechChanX = m.LechChanX
	p.LechChanY = m.LechChanY
	p.FrameVanBay = m.FrameVanBay
	p.DangMangVanBay = m.DangMangVanBay
	p.TenVanBay = m.TenVanBay
	p.Rong = m.Rong
	p.Cao = m.Cao
	p.Avatar = m.Avatar
	p.UpdatedAt = now
	p.Dirty = true
}

// AddPlayerSkeleton tạo entry rỗng khi user vào map nhưng chưa gửi move nào.
// Gọi khi player đổi map — tick kế tiếp các player khác sẽ thấy "có người mới"
// dù họ đứng yên.
func (ms *MapState) AddPlayerSkeleton(userID int32) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if _, exists := ms.players[userID]; exists {
		return
	}
	ms.players[userID] = &PlayerState{
		UserID:    userID,
		MapID:     ms.MapID,
		UpdatedAt: time.Now().UnixMilli(),
		Dirty:     false, // chưa có thông tin gì để broadcast
	}
}

func (ms *MapState) RemovePlayer(userID int32) bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if _, ok := ms.players[userID]; !ok {
		return false
	}
	delete(ms.players, userID)
	return true
}

// CollectDirty trả về snapshot copy của các player dirty và reset flag.
// Trả copy by value để giữ ngoài lock vẫn an toàn.
func (ms *MapState) CollectDirty() []PlayerState {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var dirty []PlayerState
	for _, p := range ms.players {
		if p.Dirty {
			dirty = append(dirty, *p)
			p.Dirty = false
		}
	}
	return dirty
}
