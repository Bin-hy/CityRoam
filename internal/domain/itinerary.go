package domain

const (
	ItemTypePOI  = "poi"
	ItemTypeMeal = "meal"
)

const (
	BlockTypePOI     = "poi"
	BlockTypeMeal    = "meal"
	BlockTypeTransit = "transit"
	BlockTypeRest    = "rest"
)

const (
	TransportWalk    = "walk"
	TransportTransit = "transit"
	TransportDrive   = "drive"
)

// DraftPlan is the only itinerary shape the LLM may produce. Transit,
// exact times, and costs are computed later by the Scheduler.
type DraftPlan struct {
	Days         []DraftDay    `json:"days"`
	Explanations []Explanation `json:"explanations"`
}

type DraftDay struct {
	Date  string      `json:"date"`
	Items []DraftItem `json:"items"`
}

type DraftItem struct {
	POIID       string `json:"poi_id"`
	Type        string `json:"type"`
	DurationMin int    `json:"duration_min"`
	StartHint   string `json:"start_hint"`
}

type Explanation struct {
	POIID string `json:"poi_id"`
	Text  string `json:"text"`
}

func (d DraftPlan) POIIDs() []string {
	ids := make([]string, 0)
	for _, day := range d.Days {
		for _, item := range day.Items {
			ids = append(ids, item.POIID)
		}
	}
	return ids
}

type Itinerary struct {
	City   string    `json:"city"`
	Days   []DayPlan `json:"days"`
	Totals Totals    `json:"totals"`
}

type DayPlan struct {
	Date        string  `json:"date"`
	StartTime   string  `json:"start_time"`
	EndTime     string  `json:"end_time"`
	Blocks      []Block `json:"blocks"`
	DayCostYuan int     `json:"day_cost_yuan"`
}

type Block struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	POIID       string     `json:"poi_id,omitempty"`
	Title       string     `json:"title"`
	Start       string     `json:"start"`
	End         string     `json:"end"`
	DurationMin int        `json:"duration_min"`
	CostYuan    int        `json:"cost_yuan"`
	Transport   *Transport `json:"transport,omitempty"`
	Notes       string     `json:"notes,omitempty"`
}

type Transport struct {
	Mode        string `json:"mode"`
	FromPOIID   string `json:"from_poi_id"`
	ToPOIID     string `json:"to_poi_id"`
	DistanceM   int    `json:"distance_m"`
	DurationMin int    `json:"duration_min"`
	CostYuan    int    `json:"cost_yuan"`
	Estimated   bool   `json:"estimated"`
}

type Totals struct {
	CostYuan  int     `json:"cost_yuan"`
	POICount  int     `json:"poi_count"`
	WalkingKM float64 `json:"walking_km"`
}

func (i Itinerary) DayByIndex(index int) (DayPlan, bool) {
	if index < 0 || index >= len(i.Days) {
		return DayPlan{}, false
	}
	return i.Days[index], true
}

func (i Itinerary) Clone() Itinerary {
	out := i
	out.Days = append([]DayPlan(nil), i.Days...)
	for dayIndex := range out.Days {
		out.Days[dayIndex].Blocks = append([]Block(nil), i.Days[dayIndex].Blocks...)
		for blockIndex := range out.Days[dayIndex].Blocks {
			if source := i.Days[dayIndex].Blocks[blockIndex].Transport; source != nil {
				transport := *source
				out.Days[dayIndex].Blocks[blockIndex].Transport = &transport
			}
		}
	}
	return out
}
