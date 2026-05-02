package state

import (
	"sync"
	"time"

	"github.com/DANG-PH/game-service-go/internal/shared/messages"
)

// ============================================================================
// PlayerState
// ============================================================================

// PlayerState là snapshot mới nhất của 1 player tại 1 thời điểm.
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
	DangMangVanBay bool
	TenVanBay      string
	Rong           float32
	Cao            float32
	Avatar         string

	UpdatedAt  int64 // unix milli — server timestamp khi nhận update gần nhất
	Dirty      bool  // cho broadcast tick
	RedisDirty bool  // cho Redis flush — không bị reset bởi CollectDirty
}

// ToSync convert PlayerState (struct internal của Go) sang PlayerSync (DTO gửi qua network).
//
// VISIBILITY: external (public, viết hoa)
//
// AI GỌI:
//   - ws.Ticker.tick() → mỗi tick 50ms convert tất cả dirty player thành packet
//
// MỤC ĐÍCH:
//   - Tách struct internal (PlayerState — có UpdatedAt, Dirty) khỏi DTO network (PlayerSync)
//   - Giúp việc thay đổi internal struct không ảnh hưởng wire format
//   - Tách hàm riêng để dễ test (input PlayerState, output PlayerSync)
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
		DangMangVanBay: p.DangMangVanBay,
		TenVanBay:      p.TenVanBay,
		Rong:           p.Rong,
		Cao:            p.Cao,
		Avatar:         p.Avatar,
	}
}

// ToMove convert PlayerState → PlayerMove để Ticker dùng cho Redis flush.
//
// VISIBILITY: external
//
// AI GỌI:
//   - loop.Ticker.tick() — mỗi 2s flush Redis, cần convert PlayerState sang PlayerMove
//     để tái sử dụng playerService.HandleMove
//
// MỤC ĐÍCH:
//   - Tránh duplicate logic Redis write — HandleMove đã có sẵn pipeline + dirty key
//   - Tách hàm riêng để dễ test
func (p *PlayerState) ToMove() *messages.PlayerMove {
	return &messages.PlayerMove{
		MapID:          p.MapID,
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
		DangMangVanBay: p.DangMangVanBay,
		TenVanBay:      p.TenVanBay,
		Rong:           p.Rong,
		Cao:            p.Cao,
		Avatar:         p.Avatar,
	}
}

// ============================================================================
// MapState — gom tất cả player trong 1 map
// ============================================================================

// MapState gom tất cả player đang ở trong 1 map.
// Mỗi map có lock RIÊNG (không lock toàn server) → tránh contention giữa các map.
//
// Ví dụ: player ở map "lang_tu_4" di chuyển KHÔNG block player ở map "lang_xay_da_1".
type MapState struct {
	MapID string

	mu      sync.RWMutex           // lock riêng cho map này
	players map[int32]*PlayerState // userID → state
}

// newMapState khởi tạo MapState rỗng.
//
// VISIBILITY: internal (lowercase, package-private)
//
// AI GỌI:
//   - Manager.GetOrCreateMap() — chỗ duy nhất tạo MapState mới
//
// MỤC ĐÍCH:
//   - Đóng gói việc khởi tạo (init players map empty)
//   - External code KHÔNG được tạo MapState trực tiếp — phải qua Manager
//     để đảm bảo MapState luôn được track trong Manager.maps
func newMapState(mapID string) *MapState {
	return &MapState{
		MapID:   mapID,
		players: make(map[int32]*PlayerState),
	}
}

// UpdateFromMove apply input PlayerMove từ client lên state in-memory.
//
// VISIBILITY: external
//
// AI GỌI:
//   - ws.Handler.handlePlayerMove() — mỗi packet PlayerMove từ client
//
// MỤC ĐÍCH:
//   - Update vị trí và state của player dựa trên input từ client
//   - Set Dirty = true để tick loop biết cần broadcast
//   - Tự tạo entry nếu player chưa có (player vừa join map, lần đầu di chuyển)
//     → bỏ luôn AddPlayerSkeleton, không cần gọi riêng
//
// THREAD SAFETY: lock ms.mu (write lock vì có ghi)
//
// TODO: anti-cheat validation ở đây (max speed, collision với tường, ...)
func (ms *MapState) UpdateFromMove(userID int32, m *messages.PlayerMove) {
	now := time.Now().UnixMilli()

	ms.mu.Lock()
	defer ms.mu.Unlock()

	p, ok := ms.players[userID]
	if !ok {
		// Tự tạo entry — tiện cho lần đầu player di chuyển sau khi vào map
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
	p.DangMangVanBay = m.DangMangVanBay
	p.TenVanBay = m.TenVanBay
	p.Rong = m.Rong
	p.Cao = m.Cao
	p.Avatar = m.Avatar
	p.UpdatedAt = now
	p.Dirty = true
	p.RedisDirty = true
}

// IsEmpty trả về true nếu map không còn player nào.
//
// VISIBILITY: external
//
// AI GỌI:
//   - Manager.RemovePlayerFromMap() — check sau khi xóa player để cleanup empty map
//
// MỤC ĐÍCH:
//   - Cho Manager biết khi nào nên xóa MapState khỏi Manager.maps
//     → tránh leak entry rỗng khi không còn ai trong map
//
// THREAD SAFETY: read lock (ms.mu.RLock)
func (ms *MapState) IsEmpty() bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.players) == 0
}

// RemovePlayer xóa player khỏi map.
//
// VISIBILITY: external
//
// AI GỌI:
//   - Manager.RemovePlayerFromMap() — wrapper qua Manager (cách phổ biến)
//   - Có thể gọi trực tiếp khi caller đã có sẵn ref *MapState (ít dùng)
//
// MỤC ĐÍCH:
//   - Xóa player khỏi map khi: disconnect, đổi map, bị kick
//   - Trả về true/false để caller biết player có thực sự tồn tại không
//
// LƯU Ý: Hàm này KHÔNG cleanup empty map khỏi Manager.
//
//	Nếu cần cleanup → dùng Manager.RemovePlayerFromMap thay vì hàm này trực tiếp.
//
// THREAD SAFETY: write lock (ms.mu.Lock)
func (ms *MapState) RemovePlayer(userID int32) bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if _, ok := ms.players[userID]; !ok {
		return false
	}
	delete(ms.players, userID)
	return true
}

// CollectDirty thu thập tất cả player có Dirty = true, reset flag, trả slice copy.
//
// VISIBILITY: external
//
// AI GỌI:
//   - ws.Ticker.tick() — mỗi 50ms để biết player nào cần broadcast
//
// MỤC ĐÍCH:
//   - Tick loop chỉ broadcast player có thay đổi thực sự (tiết kiệm bandwidth)
//   - Reset Dirty về false sau khi collect → tick sau chỉ broadcast nếu có update mới
//
// LƯU Ý:
//   - Trả COPY by value (PlayerState, không phải *PlayerState)
//     → caller có thể iterate ngoài lock an toàn
//     → caller có thể modify copy mà không ảnh hưởng state thật
//   - Nếu trả pointer → caller modify ngoài lock = race condition
//
// THREAD SAFETY: write lock (vì có ghi Dirty = false)
func (ms *MapState) CollectDirty() []PlayerState {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	dirty := make([]PlayerState, 0, len(ms.players))
	for _, p := range ms.players {
		if p.Dirty {
			dirty = append(dirty, *p)
			p.Dirty = false
		}
	}
	return dirty
}

// CollectDirtyBoth thu thập dirty players cho cả broadcast lẫn Redis flush trong 1 lần lock.
// Tránh acquire lock 2 lần liên tiếp khi tick trùng với flush interval.
func (ms *MapState) CollectDirtyBoth() (broadcast []PlayerState, redis []PlayerState) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	for _, p := range ms.players {
		if p.Dirty {
			broadcast = append(broadcast, *p)
			p.Dirty = false
		}
		if p.RedisDirty {
			redis = append(redis, *p)
			p.RedisDirty = false
		}
	}
	return
}

// ============================================================================
// Manager — registry của tất cả MapState đang active
// ============================================================================

// Manager quản lý lifecycle của tất cả MapState đang active trên server.
//
// PHÂN CHIA LOCK:
//   - Manager có lock RIÊNG (m.mu) — chỉ acquire khi tạo/xóa MapState
//   - Mỗi MapState có lock RIÊNG (ms.mu) — cho operation trong nội bộ map
//   - 2 layer lock độc lập → high concurrency
//
// LIFECYCLE:
//   - GetOrCreateMap() → tạo MapState khi player đầu tiên vào map
//   - RemovePlayerFromMap() → xóa player + cleanup MapState rỗng
type Manager struct {
	mu   sync.RWMutex
	maps map[string]*MapState // mapID → state
}

// NewManager khởi tạo Manager rỗng.
//
// VISIBILITY: external
//
// AI GỌI:
//   - main.go (cmd/server/main.go) — khởi tạo 1 lần lúc startup
//
// MỤC ĐÍCH:
//   - Entry point để tạo Manager
//   - Khởi tạo maps là empty map (không nil) để các method khác safe
func NewManager() *Manager {
	return &Manager{maps: make(map[string]*MapState)}
}

// GetOrCreateMap trả về MapState của mapID. Nếu chưa có → tạo mới và lưu vào Manager.
//
// VISIBILITY: external
//
// AI GỌI:
//   - ws.Handler.handlePlayerMove() — mỗi packet cần đảm bảo map tồn tại trước khi update
//
// MỤC ĐÍCH:
//   - Lazy initialization: map chỉ tạo khi có player thật sự vào
//   - Không cần "preregister" map nào — flexible với map mới
//
// PATTERN: Double-check locking
//   - Optimistic RLock đầu tiên (case chung: map đã tồn tại) — nhanh
//   - Nếu chưa có → upgrade lên write Lock, double-check (vì có thể goroutine khác
//     vừa tạo trong khoảng time giữa unlock RLock và acquire Lock)
//   - Tránh race condition khi 2 goroutine cùng tạo 1 map
//
// THREAD SAFETY: tự handle lock (RLock → fast path, Lock → slow path)
func (m *Manager) GetOrCreateMap(mapID string) *MapState {
	// Fast path: map đã tồn tại
	m.mu.RLock()
	ms, ok := m.maps[mapID]
	m.mu.RUnlock()
	if ok {
		return ms
	}

	// Slow path: tạo mới với write lock
	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check — có thể goroutine khác vừa tạo
	if ms, ok := m.maps[mapID]; ok {
		return ms
	}
	ms = newMapState(mapID)
	m.maps[mapID] = ms
	return ms
}

// GetMap chỉ lookup, KHÔNG tạo mới. Trả ok=false nếu không tồn tại.
//
// VISIBILITY: external
//
// AI GỌI:
//   - Manager.RemovePlayerFromMap() — lookup map để xóa player
//   - Có thể gọi từ Handler/Ticker khi cần check map tồn tại trước khi action
//
// MỤC ĐÍCH:
//   - Khác GetOrCreateMap: không side-effect (không tạo map)
//   - Dùng khi muốn biết "map này có không?" mà không muốn tạo mới
//
// THREAD SAFETY: read lock (m.mu.RLock) — chỉ đọc
func (m *Manager) GetMap(mapID string) (*MapState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ms, ok := m.maps[mapID]
	return ms, ok
}

// AllMaps trả slice copy tất cả MapState đang active.
//
// VISIBILITY: external
//
// AI GỌI:
//   - ws.Ticker.tick() — mỗi 50ms iterate qua tất cả map để collect dirty + broadcast
//
// MỤC ĐÍCH:
//   - Tick loop cần biết danh sách map để xử lý từng map
//   - Trả slice copy → caller iterate ngoài lock an toàn
//
// LƯU Ý:
//   - Slice là copy nhưng phần tử vẫn là *MapState (pointer)
//   - Nếu MapState bị xóa khỏi Manager.maps trong khi caller đang iterate
//     → pointer vẫn valid, nhưng data có thể outdated
//   - Trade-off này chấp nhận được vì tick loop có thể xử lý "MapState đã xóa"
//     mà không crash (CollectDirty trả slice rỗng → skip)
//
// THREAD SAFETY: read lock
func (m *Manager) AllMaps() []*MapState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*MapState, 0, len(m.maps))
	for _, ms := range m.maps {
		result = append(result, ms)
	}
	return result
}

// RemovePlayerFromMap xóa player khỏi map (lookup theo mapID), tự cleanup empty map.
//
// VISIBILITY: external
//
// AI GỌI:
//   - ws.Hub.unregister() — khi conn disconnect (read loop return)
//   - ws.Handler.handlePlayerMove() — khi player đổi map (cleanup map cũ)
//
// MỤC ĐÍCH:
//   - Wrapper convenience: caller chỉ cần biết mapID string, không cần giữ ref *MapState
//   - Tự động cleanup empty map khỏi Manager.maps → tránh leak entry rỗng
//     (sau khi remove player cuối cùng → map đó vô dụng → xóa luôn)
//
// KHÁC MapState.RemovePlayer:
//   - MapState.RemovePlayer chỉ xóa player, KHÔNG cleanup empty map
//   - Manager.RemovePlayerFromMap = MapState.RemovePlayer + cleanup map rỗng
//     → Nên dùng hàm này trừ khi có lý do cụ thể không muốn cleanup empty map
//
// PATTERN: Double-check locking (lần thứ 2)
//   - IsEmpty check không lock Manager → có thể có race với GetOrCreateMap
//     (player khác vừa join giữa lúc check IsEmpty và acquire write lock)
//   - Double-check sau khi acquire lock để chắc chắn map vẫn rỗng trước khi xóa
//
// THREAD SAFETY: tự handle lock cho cả MapState và Manager
func (m *Manager) RemovePlayerFromMap(mapID string, userID int32) {
	ms, ok := m.GetMap(mapID)
	if !ok {
		return
	}
	ms.RemovePlayer(userID)

	if ms.IsEmpty() {
		m.mu.Lock()
		defer m.mu.Unlock()
		// Double-check sau khi acquire write lock
		// (có thể player khác vừa join giữa check IsEmpty và acquire lock)
		if ms.IsEmpty() {
			delete(m.maps, mapID)
		}
	}
}

// Xem player có tồn tại trên map nữa không
func (m *Manager) PlayerExistsInMap(mapID string, userID int32) bool {
	ms, ok := m.GetMap(mapID)
	if !ok {
		return false
	}
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	_, exists := ms.players[userID]
	return exists
}
