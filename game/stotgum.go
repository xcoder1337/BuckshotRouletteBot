package game

import (
	"math/rand"
)

// ShellType — тип патрона
type ShellType int

const (
	ShellBlank ShellType = iota // 🔵 Холостой
	ShellLive                   // 🔴 Боевой
)

func (s ShellType) Emoji() string {
	if s == ShellLive {
		return "🔴"
	}
	return "🔵"
}

func (s ShellType) Name() string {
	if s == ShellLive {
		return "🔴 Боевой"
	}
	return "🔵 Холостой"
}

// Shotgun — дробовик
type Shotgun struct {
	Magazine []ShellType
	SawedOff bool // ножовка применена
}

// NewShotgun создаёт дробовик с случайной обоймой
func NewShotgun(minShells, maxShells int) *Shotgun {
	count := minShells + rand.Intn(maxShells-minShells+1)
	if count < 2 {
		count = 2
	}
	if count > 8 {
		count = 8
	}

	magazine := make([]ShellType, count)

	// Гарантируем минимум 1 боевой и 1 холостой
	liveCount := 1 + rand.Intn(count-1)
	for i := 0; i < count; i++ {
		if i < liveCount {
			magazine[i] = ShellLive
		} else {
			magazine[i] = ShellBlank
		}
	}

	// Перемешиваем
	rand.Shuffle(len(magazine), func(i, j int) {
		magazine[i], magazine[j] = magazine[j], magazine[i]
	})

	return &Shotgun{
		Magazine: magazine,
		SawedOff: false,
	}
}

// CurrentShell — текущий патрон (первый в обойме)
func (s *Shotgun) CurrentShell() ShellType {
	if len(s.Magazine) == 0 {
		return ShellBlank
	}
	return s.Magazine[0]
}

// Shoot — выстрел, возвращает тип патрона
func (s *Shotgun) Shoot() ShellType {
	if len(s.Magazine) == 0 {
		return ShellBlank
	}
	shell := s.Magazine[0]
	s.Magazine = s.Magazine[1:]
	return shell
}

// Rack — извлечь текущий патрон (пиво)
func (s *Shotgun) Rack() ShellType {
	return s.Shoot()
}

// InvertCurrent — инвертировать текущий патрон
func (s *Shotgun) InvertCurrent() {
	if len(s.Magazine) == 0 {
		return
	}
	if s.Magazine[0] == ShellLive {
		s.Magazine[0] = ShellBlank
	} else {
		s.Magazine[0] = ShellLive
	}
}

// IsEmpty — пуста ли обойма
func (s *Shotgun) IsEmpty() bool {
	return len(s.Magazine) == 0
}

// LiveCount — количество боевых
func (s *Shotgun) LiveCount() int {
	count := 0
	for _, shell := range s.Magazine {
		if shell == ShellLive {
			count++
		}
	}
	return count
}

// BlankCount — количество холостых
func (s *Shotgun) BlankCount() int {
	count := 0
	for _, shell := range s.Magazine {
		if shell == ShellBlank {
			count++
		}
	}
	return count
}

// GetRandomShellInfo — для телефона, возвращает позицию и тип
func (s *Shotgun) GetRandomShellInfo() (int, ShellType) {
	if len(s.Magazine) <= 1 {
		return 0, s.CurrentShell()
	}
	// Выбираем случайную позицию (не первую, чтобы было интереснее)
	pos := 1 + rand.Intn(len(s.Magazine)-1)
	return pos + 1, s.Magazine[pos] // +1 для человекочитаемой нумерации
}

// Damage — урон текущего выстрела
func (s *Shotgun) Damage() int {
	if s.SawedOff {
		return 2
	}
	return 1
}
