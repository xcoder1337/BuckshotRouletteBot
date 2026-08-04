package game

import "math/rand"

// DealerAI — ИИ для одиночной игры (дилер)
type DealerAI struct{}

// DecideAction — решение ИИ
func (ai *DealerAI) DecideAction(g *Game) (useItem bool, itemType ItemType, shootSelf bool) {
	dealer := g.CurrentPlayer()
	player := g.OpponentPlayer()

	// Приоритет использования предметов
	for _, item := range dealer.Items {
		switch item.Type {
		case ItemMagnifyingGlass:
			if g.KnownShell == nil {
				return true, ItemMagnifyingGlass, false
			}

		case ItemBurnerPhone:
			if g.KnownShell == nil && len(g.Shotgun.Magazine) > 2 {
				return true, ItemBurnerPhone, false
			}

		case ItemCigarettes:
			if dealer.HP < dealer.MaxHP {
				return true, ItemCigarettes, false
			}

		case ItemHandcuffs:
			if !player.Handcuffed {
				return true, ItemHandcuffs, false
			}

		case ItemHandsaw:
			if !g.SawedOff && g.KnownShell != nil && *g.KnownShell == ShellLive {
				return true, ItemHandsaw, false
			}

		case ItemBeer:
			if g.KnownShell != nil && *g.KnownShell == ShellBlank {
				return true, ItemBeer, false
			}

		case ItemInverter:
			if g.KnownShell != nil && *g.KnownShell == ShellBlank {
				return true, ItemInverter, false
			}

		case ItemExpiredMedicine:
			if dealer.HP <= 1 {
				return true, ItemExpiredMedicine, false
			}
		}
	}

	// Решение о выстреле
	if g.KnownShell != nil {
		if *g.KnownShell == ShellBlank {
			// Известно что холостой — в себя
			return false, 0, true
		}
		// Известно что боевой — в игрока
		return false, 0, false
	}

	// Если больше холостых, стреляем в себя
	if g.Shotgun.BlankCount() > g.Shotgun.LiveCount() {
		return false, 0, true
	}

	// Если поровну или больше боевых — в игрока
	if g.Shotgun.LiveCount() >= g.Shotgun.BlankCount() {
		return false, 0, false
	}

	// Случайно
	return false, 0, rand.Intn(2) == 0
}
