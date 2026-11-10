package game

import "math/rand"

// DealerAI — ИИ для одиночной игры (дилер)
type DealerAI struct{}

// DecideAction — решение ИИ
func (ai *DealerAI) DecideAction(g *Game) (useItem bool, itemType ItemType, shootSelf bool) {
	dealer := g.CurrentPlayer()
	player := g.OpponentPlayer()

	// Дилер играет только по СВОЕЙ разведке. Раньше он читал общий g.KnownShell,
	// куда писал и игрок: ИИ видел чужую лупу и играл безошибочно.
	known := dealer.KnownShell
	if known == nil {
		if shell, ok := dealer.PhoneKnown[1]; ok {
			// Телефон раскрыл патрон в стволе — для дилера это то же знание.
			known = &shell
		}
	}

	// Приоритет использования предметов
	for _, item := range dealer.Items {
		switch item.Type {
		case ItemMagnifyingGlass:
			if known == nil {
				return true, ItemMagnifyingGlass, false
			}

		case ItemBurnerPhone:
			if known == nil && len(dealer.PhoneKnown) == 0 && len(g.Shotgun.Magazine) > 2 {
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
			if !g.SawedOff && known != nil && *known == ShellLive {
				return true, ItemHandsaw, false
			}

		case ItemBeer:
			if known != nil && *known == ShellBlank {
				return true, ItemBeer, false
			}

		case ItemInverter:
			if known != nil && *known == ShellBlank {
				return true, ItemInverter, false
			}

		case ItemExpiredMedicine:
			if dealer.HP <= 1 {
				return true, ItemExpiredMedicine, false
			}
		}
	}

	// Решение о выстреле
	if known != nil {
		if *known == ShellBlank {
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
