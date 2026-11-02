package cdp

import "context"

// Only completed, itemized restaurant orders contribute to dish popularity.
// Separate currencies so monetary totals are never mixed across currencies.
const popularFoodSQL = `WITH order_items AS (
 SELECT e.id AS order_id,json_extract(e.properties_json,'$.currency') AS currency,
 CASE WHEN item.type='object' THEN item.value ELSE '{}' END AS item_json
 FROM events e, json_each(CASE WHEN json_type(e.properties_json,'$.items')='array'
 THEN json_extract(e.properties_json,'$.items') ELSE '[]' END) item
 WHERE e.event_key='restaurant_order' AND json_extract(e.properties_json,'$.status')='completed'
), food AS (
 SELECT order_id,currency,
 json_extract(item_json,'$.dish_id') AS dish_id,
 json_extract(item_json,'$.dish_name') AS dish_name,
 json_extract(item_json,'$.dish_name_th') AS dish_name_th,
 json_extract(item_json,'$.category') AS category,
 json_extract(item_json,'$.quantity') AS quantity,
 json_extract(item_json,'$.line_total') AS line_total
 FROM order_items
 WHERE currency IS NOT NULL
 AND json_type(item_json,'$.quantity')='integer'
 AND json_extract(item_json,'$.quantity')>0
 AND json_type(item_json,'$.line_total') IN ('integer','real')
 AND json_extract(item_json,'$.line_total')>=0
 AND json_extract(item_json,'$.dish_id') IS NOT NULL
), ranked AS (
 SELECT dish_id,MAX(dish_name) AS dish_name,MAX(dish_name_th) AS dish_name_th,
 MAX(category) AS category,currency,SUM(quantity) AS quantity_sold,
 COUNT(DISTINCT order_id) AS order_count,ROUND(SUM(line_total),2) AS item_revenue
 FROM food GROUP BY dish_id,currency
)`

// ListPopularFood ranks dishes by units sold, followed by order count and ID.
// Item revenue excludes order-level service charges and VAT.
func (s *Service) ListPopularFood(ctx context.Context, in PageInput) (*PageResult, error) {
	db, err := s.manager.Get(ctx, in.DatabaseID)
	if err != nil {
		return nil, err
	}
	total, err := countQuery(ctx, db, popularFoodSQL+` SELECT count(*) FROM ranked`)
	if err != nil {
		return nil, err
	}
	window := newPaginationWindow(in.Page, in.PageSize, total)
	rows, err := queryMaps(ctx, db, popularFoodSQL+` SELECT * FROM ranked ORDER BY quantity_sold DESC,order_count DESC,dish_id,currency LIMIT ? OFFSET ?`, window.limit, window.offset)
	if err != nil {
		return nil, err
	}
	return pageResult(rows, window), nil
}
