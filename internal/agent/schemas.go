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
	        "hard": {"type": "boolean"}
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
)
