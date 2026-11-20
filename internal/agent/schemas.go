package agent

import "encoding/json"

// Schema is passed to the LLM prompt and used by tests to validate decoding
// shapes. The server does not enforce it; nodes still reject bad output.
var (
	ConstraintSchema = json.RawMessage(`{
	  "$schema": "https://json-schema.org/draft/2020-12/schema",
	  "type": "object",
	  "additionalProperties": false,
	  "properties": {
	    "city": {"type": "string", "description": "城市 slug；MVP 仅 beijing，未指定时为空"},
	    "days": {"type": "integer", "minimum": 0, "maximum": 4, "description": "1-4；未指定时 0"},
	    "start_date": {"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
	    "party_size": {"type": "integer", "minimum": 0, "maximum": 20},
	    "budget": {
	      "type": "object",
	      "additionalProperties": false,
	      "properties": {
	        "total_yuan": {"type": "integer", "minimum": 0},
	        "hard": {"type": "boolean"},
	        "include_lodging": {"type": "boolean", "description": "false 表示明确不含住宿；省略或 true 表示默认含"}
	      }
	    },
	    "lodging": {
	      "type": "object",
	      "additionalProperties": false,
	      "properties": {
	        "mentioned": {"type": "boolean", "description": "用户是否明确提到酒店、民宿、住宿或住哪里"},
	        "hotels": {"type": "array", "items": {"type": "string"}, "description": "只抽取用户原话中指定的酒店名"},
	        "area": {"type": "string", "description": "用户希望住宿的区域"},
	        "max_price_per_night_yuan": {"type": "integer", "minimum": 0}
	      }
	    },
	    "daily": {
	      "type": "object",
	      "additionalProperties": false,
	      "properties": {
	        "max_hours": {"type": "integer", "minimum": 1, "maximum": 16},
	        "start_time": {"type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$"},
	        "end_time": {"type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$"}
	      }
	    },
	    "meals": {
	      "type": "object",
	      "additionalProperties": false,
	      "properties": {
	        "enabled": {"type": "boolean"},
	        "lunch_window": {"type": "array", "items": {"type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$"}, "minItems": 2, "maxItems": 2},
	        "dinner_window": {"type": "array", "items": {"type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$"}, "minItems": 2, "maxItems": 2},
	        "budget_per_meal_yuan": {"type": "integer", "minimum": 0}
	      }
	    },
	    "pace": {"type": "string", "enum": ["relaxed", "normal", "packed"]},
	    "interests": {"type": "array", "items": {"type": "string"}},
	    "must_visit": {"type": "array", "items": {"type": "string"}},
	    "avoid": {"type": "array", "items": {"type": "string"}},
	    "walking": {
	      "type": "object",
	      "additionalProperties": false,
	      "properties": {
	        "max_km_per_day": {"type": "number", "minimum": 0, "maximum": 30}
	      }
	    },
	    "weather": {
	      "type": "object",
	      "additionalProperties": false,
	      "properties": {
	        "rain_aware": {"type": "boolean"}
	      }
	    },
	    "hard": {"type": "array", "items": {"type": "string"}},
	    "soft": {"type": "array", "items": {"type": "string"}},
	    "missing": {"type": "array", "items": {"type": "string", "enum": ["city", "days"]}}
	  }
	}`)

	DraftPlanSchema = json.RawMessage(`{
	  "$schema": "https://json-schema.org/draft/2020-12/schema",
	  "type": "object",
	  "additionalProperties": false,
	  "required": ["days"],
	  "properties": {
	    "days": {
	      "type": "array",
	      "minItems": 1,
	      "maxItems": 4,
	      "items": {
	        "type": "object",
	        "additionalProperties": false,
	        "required": ["items"],
	        "properties": {
	          "date": {"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
	          "items": {
	            "type": "array",
	            "minItems": 1,
	            "maxItems": 8,
	            "items": {
	              "type": "object",
	              "additionalProperties": false,
	              "required": ["poi_id", "type", "duration_min"],
	              "properties": {
	                "poi_id": {"type": "string", "minLength": 1},
	                "type": {"type": "string", "enum": ["poi", "meal"]},
	                "duration_min": {"type": "integer", "minimum": 15, "maximum": 480},
	                "start_hint": {"type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$"}
	              }
	            }
	          }
	        }
	      }
	    },
	    "explanations": {
	      "type": "array",
	      "items": {
	        "type": "object",
	        "additionalProperties": false,
	        "required": ["poi_id", "text"],
	        "properties": {
	          "poi_id": {"type": "string", "minLength": 1},
	          "text": {"type": "string", "minLength": 1}
	        }
	      }
	    }
	  }
	}`)

	DayPatchSchema = json.RawMessage(`{
	  "$schema": "https://json-schema.org/draft/2020-12/schema",
	  "type": "object",
	  "additionalProperties": false,
	  "required": ["day_index", "items", "reason"],
	  "properties": {
	    "day_index": {"type": "integer", "minimum": 0, "maximum": 3},
	    "items": {
	      "type": "array",
	      "minItems": 1,
	      "maxItems": 8,
	      "items": {
	        "type": "object",
	        "additionalProperties": false,
	        "required": ["poi_id", "type", "duration_min"],
	        "properties": {
	          "poi_id": {"type": "string", "minLength": 1},
	          "type": {"type": "string", "enum": ["poi", "meal"]},
	          "duration_min": {"type": "integer", "minimum": 15, "maximum": 480},
	          "start_hint": {"type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$"}
	        }
	      }
	    },
	    "reason": {"type": "string", "minLength": 1}
	  }
	}`)

	AdjustmentIntentSchema = json.RawMessage(`{
	  "$schema": "https://json-schema.org/draft/2020-12/schema",
	  "type": "object",
	  "additionalProperties": false,
	  "required": ["scope", "affected_days"],
	  "properties": {
	    "scope": {"type": "string", "enum": ["day", "block", "poi", "pace", "budget", "unknown"]},
	    "affected_days": {
	      "type": "array",
	      "maxItems": 4,
	      "items": {"type": "integer", "minimum": 0, "maximum": 3}
	    },
	    "summary": {"type": "string", "maxLength": 200}
	  }
	}`)

	ResearchBriefSchema = json.RawMessage(`{
	  "$schema": "https://json-schema.org/draft/2020-12/schema",
	  "type": "object",
	  "additionalProperties": false,
	  "required": ["status"],
	  "properties": {
	    "status": {"type": "string", "enum": ["ok", "degraded", "skipped"]},
	    "accommodation": {
	      "type": "object",
	      "additionalProperties": false,
	      "properties": {
	        "mentioned": {"type": "boolean"},
	        "specified": {"type": "array", "items": {"type": "string"}},
	        "selected": {"$ref": "#/$defs/hotel"},
	        "options": {"type": "array", "maxItems": 5, "items": {"$ref": "#/$defs/hotel"}},
	        "anchor_days": {"type": "array", "maxItems": 4, "items": {"type": "integer", "minimum": 0, "maximum": 3}},
	        "nights": {"type": "integer", "minimum": 0},
	        "nightly_yuan": {"type": "integer", "minimum": 0},
	        "total_yuan": {"type": "integer", "minimum": 0},
	        "price_estimated": {"type": "boolean"},
	        "include_in_budget": {"type": "boolean"},
	        "reason": {"type": "string", "maxLength": 200}
	      }
	    },
	    "guides": {
	      "type": "array",
	      "items": {
	        "type": "object",
	        "additionalProperties": false,
	        "required": ["poi_id", "weight", "mentions"],
	        "properties": {
	          "poi_id": {"type": "string"},
	          "weight": {"type": "number", "minimum": 0, "maximum": 1},
	          "mentions": {"type": "integer", "minimum": 0},
	          "note": {"type": "string", "maxLength": 120},
	          "citations": {
	            "type": "array",
	            "maxItems": 3,
	            "items": {
	              "type": "object",
	              "additionalProperties": false,
	              "required": ["title", "url"],
	              "properties": {
	                "title": {"type": "string"},
	                "url": {"type": "string"},
	                "site": {"type": "string"},
	                "published_at": {"type": "string"},
	                "authority": {"type": "integer", "minimum": 0}
	              }
	            }
	          }
	        }
	      }
	    },
	    "notes": {"type": "array", "items": {"type": "string"}},
	    "warnings": {"type": "array", "items": {"type": "string"}},
	    "tool_calls": {"type": "integer", "minimum": 0},
	    "duration_ms": {"type": "integer", "minimum": 0}
	  },
	  "$defs": {
	    "hotel": {
	      "type": "object",
	      "additionalProperties": false,
	      "required": ["id", "name", "lat", "lng", "price_per_night_yuan", "price_estimated", "provider", "fetched_at"],
	      "properties": {
	        "id": {"type": "string"},
	        "name": {"type": "string"},
	        "address": {"type": "string"},
	        "city": {"type": "string"},
	        "lat": {"type": "number"},
	        "lng": {"type": "number"},
	        "star": {"type": "integer", "minimum": 0, "maximum": 5},
	        "score": {"type": "number", "minimum": 0, "maximum": 5},
	        "price_per_night_yuan": {"type": "integer", "minimum": 0},
	        "price_estimated": {"type": "boolean"},
	        "booking_url": {"type": "string"},
	        "provider": {"type": "string"},
	        "fetched_at": {"type": "string"}
	      }
	    }
	  }
	}`)
)
