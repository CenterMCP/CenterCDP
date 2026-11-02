package cdp

import (
	"fmt"
	"strings"
)

const sampleHotelName = "Serenity Pearl Hotel"

// Curated fictional combinations keep fixtures repeatable without a faker
// dependency. Reserved example domains avoid using real guests' mailboxes.
func hotelGuestIdentity(n int) (name, email string) {
	first := []string{
		"Nattapong", "Pimchanok", "Thanawat", "Sirin", "Kritsada", "Araya", "Pawat", "Nicha", "Chanon", "Waranya",
		"Teerapat", "Kanyarat", "Suphachai", "Sasithorn", "Anan", "Nalin", "Phurin", "Patcharin", "Worawut", "Benjawan",
		"Emily", "Oliver", "Sophie", "James", "Charlotte", "Liam", "Amelia", "Noah", "Isabella", "Lucas",
		"Grace", "Ethan", "Hannah", "Daniel", "Chloe", "Alexander", "Mia", "Benjamin", "Lily", "Samuel",
	}
	last := []string{
		"Srisawat", "Wattanakul", "Saetang", "Chaiyaporn", "Rattanapong", "Suksamran", "Thongsuk", "Charoenkit", "Boonsri", "Phanich",
		"Suwanrat", "Kittipong", "Prasertkul", "Maneewan", "Wongchai", "Sangthong", "Phromsak", "Intarawong", "Thaworn", "Lertwong",
		"Wilson", "Bennett", "Morgan", "Harrison", "Clarke", "Anderson", "Mitchell", "Walker", "Thompson", "Reed",
		"Parker", "Collins", "Hayes", "Brooks", "Foster", "Sullivan", "Carter", "Hughes", "Edwards", "Turner",
	}
	index := (n - 1) % len(first)
	group := index / 20 * 20
	surname := last[group+(index%20+(n-1)/len(first)*7)%20]
	given := first[index]
	name = given + " " + surname
	local := strings.ToLower(given + "." + surname)
	if n%3 == 0 {
		local = strings.ToLower(surname + "." + given)
	}
	domains := []string{"example.com", "example.net", "example.org"}
	return name, local + "@" + domains[(n-1)%len(domains)]
}

type hotelDish struct {
	ID, Name, ThaiName, Category string
	Price                        int // THB, excluding service charge and VAT
}

var hotelMenu = []hotelDish{
	{"pad-thai-prawn", "Pad Thai with Prawns", "ผัดไทยกุ้งสด", "main", 280},
	{"tom-yum-goong", "Tom Yum Goong", "ต้มยำกุ้ง", "soup", 320},
	{"green-curry-chicken", "Green Curry with Chicken", "แกงเขียวหวานไก่", "main", 260},
	{"pad-kra-pao-beef", "Holy Basil Beef with Rice", "ข้าวกะเพราเนื้อ", "main", 290},
	{"mango-sticky-rice", "Mango Sticky Rice", "ข้าวเหนียวมะม่วง", "dessert", 180},
	{"som-tam-thai", "Thai Papaya Salad", "ส้มตำไทย", "salad", 160},
	{"grilled-seabass", "Grilled Seabass with Lime Sauce", "ปลากะพงย่างซอสมะนาว", "main", 480},
	{"chicken-satay", "Chicken Satay", "สะเต๊ะไก่", "starter", 190},
	{"thai-iced-tea", "Thai Iced Tea", "ชาไทยเย็น", "beverage", 95},
	{"coconut-water", "Fresh Coconut Water", "น้ำมะพร้าวสด", "beverage", 120},
}

func hotelRestaurantOrder(n int, stayID, room string) map[string]any {
	// Weight the rotating main course toward Pad Thai, with shared desserts and
	// beverages, so popularity reflects repeated dishes rather than unique orders.
	mains := []int{0, 0, 1, 0, 2, 3, 1, 6}
	choices := []int{mains[(n-1)%len(mains)], 8 + n%2}
	if n%2 == 0 {
		choices = append(choices, 4)
	}
	if n%3 == 0 {
		choices = append(choices, 5)
	}
	if n%5 == 0 {
		choices = append(choices, 7)
	}
	items := make([]map[string]any, 0, len(choices))
	subtotal := 0
	for index, dishIndex := range choices {
		dish := hotelMenu[dishIndex]
		quantity := 1
		if index == 0 && n%4 == 0 {
			quantity = 2
		}
		lineTotal := dish.Price * quantity
		subtotal += lineTotal
		items = append(items, map[string]any{"dish_id": dish.ID, "dish_name": dish.Name, "dish_name_th": dish.ThaiName, "category": dish.Category, "quantity": quantity, "unit_price": dish.Price, "line_total": lineTotal})
	}
	// Integer satang arithmetic keeps receipt totals exact to two decimal places.
	subtotalSatang := subtotal * 100
	serviceSatang := subtotalSatang / 10
	vatSatang := ((subtotalSatang+serviceSatang)*7 + 50) / 100
	return map[string]any{
		"stay_id": stayID, "room_number": room, "order_id": fmt.Sprintf("SPH-ORDER-%03d", n),
		"restaurant_name": "Pearl Kitchen", "table_number": fmt.Sprintf("T%02d", 1+(n-1)%12),
		"meal_period": []string{"lunch", "dinner"}[(n-1)%2], "currency": "THB", "status": "completed",
		"items": items, "subtotal": float64(subtotalSatang) / 100, "service_charge": float64(serviceSatang) / 100,
		"tax": float64(vatSatang) / 100, "amount": float64(subtotalSatang+serviceSatang+vatSatang) / 100,
	}
}
