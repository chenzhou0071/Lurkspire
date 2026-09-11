// store_test.go — 装备目录：种子一致/按序/查件/不存在防御
package store

import "testing"

func TestMemStore_ListEquipments(t *testing.T) {
	m := NewMemStore()
	list, err := m.ListEquipments()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 5 { // 无 + 4 件
		t.Fatalf("want 5 equipments, got %d", len(list))
	}
	// 按 id 升序（= 展示顺序）
	for i, e := range list {
		if e.ID != i {
			t.Fatalf("order broken at %d: id=%d", i, e.ID)
		}
	}
	// 种子内容与 db/schema.sql 一致
	if list[1].Name != "轻盈之靴" || list[1].EffectType != "air_jump" || list[1].EffectValue != 0.3 {
		t.Fatalf("equip 1 wrong: %+v", list[1])
	}
	if list[4].Name != "守护徽章" || list[4].EffectType != "block_max" || list[4].EffectValue != 10 {
		t.Fatalf("equip 4 wrong: %+v", list[4])
	}
}

func TestMemStore_GetEquipment(t *testing.T) {
	m := NewMemStore()
	e, err := m.GetEquipment(2)
	if err != nil || e.Name != "疾风护腕" || e.EffectValue != 1 {
		t.Fatalf("get 2 wrong: %+v err=%v", e, err)
	}
	if _, err := m.GetEquipment(99); err != ErrNotFound {
		t.Fatalf("missing equip: want ErrNotFound, got %v", err)
	}
}
