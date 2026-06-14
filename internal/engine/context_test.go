package engine

import (
	"fmt"
	"testing"
)

func TestContext_SetAndGet(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		value  interface{}
		getKey string
		want   interface{}
		wantOk bool
	}{
		{
			name:   "simple string",
			key:    "token",
			value:  "abc123",
			getKey: "token",
			want:   "abc123",
			wantOk: true,
		},
		{
			name:   "integer",
			key:    "count",
			value:  42,
			getKey: "count",
			want:   42,
			wantOk: true,
		},
		{
			name:   "missing key",
			key:    "exists",
			value:  true,
			getKey: "missing",
			want:   nil,
			wantOk: false,
		},
		{
			name:   "nested set via dot notation",
			key:    "response.body.id",
			value:  "xyz",
			getKey: "response.body.id",
			want:   "xyz",
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext()
			ctx.Set(tt.key, tt.value)

			got, ok := ctx.Get(tt.getKey)
			if ok != tt.wantOk {
				t.Errorf("Get(%q) ok = %v, want %v", tt.getKey, ok, tt.wantOk)
			}
			if tt.wantOk && got != tt.want {
				t.Errorf("Get(%q) = %v, want %v", tt.getKey, got, tt.want)
			}
		})
	}
}

func TestContext_ResolveString(t *testing.T) {
	tests := []struct {
		name  string
		vars  map[string]interface{}
		input string
		want  string
	}{
		{
			name:  "no variables",
			vars:  nil,
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "single variable",
			vars:  map[string]interface{}{"name": "flowtest"},
			input: "hello ${name}",
			want:  "hello flowtest",
		},
		{
			name:  "multiple variables",
			vars:  map[string]interface{}{"host": "localhost", "port": 8080},
			input: "http://${host}:${port}/api",
			want:  "http://localhost:8080/api",
		},
		{
			name:  "unresolved variable stays",
			vars:  map[string]interface{}{"a": "1"},
			input: "${a} ${b}",
			want:  "1 ${b}",
		},
		{
			name:  "bearer token pattern",
			vars:  map[string]interface{}{"token": "jwt-abc"},
			input: "Bearer ${token}",
			want:  "Bearer jwt-abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext()
			for k, v := range tt.vars {
				ctx.Set(k, v)
			}
			got := ctx.ResolveString(tt.input)
			if got != tt.want {
				t.Errorf("ResolveString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestContext_EvalBool(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[string]interface{}
		expr    string
		want    bool
		wantErr bool
	}{
		{
			name: "simple equality true",
			vars: map[string]interface{}{"status": 200},
			expr: "status == 200",
			want: true,
		},
		{
			name: "simple equality false",
			vars: map[string]interface{}{"status": 404},
			expr: "status == 200",
			want: false,
		},
		{
			name: "string comparison",
			vars: map[string]interface{}{"name": "test"},
			expr: "name == 'test'",
			want: true,
		},
		{
			name: "greater than",
			vars: map[string]interface{}{"total": 10},
			expr: "total > 5",
			want: true,
		},
		{
			name: "not equal",
			vars: map[string]interface{}{"token": "abc"},
			expr: "token != ''",
			want: true,
		},
		{
			name: "nested map access",
			vars: map[string]interface{}{
				"response": map[string]interface{}{
					"status": 200,
				},
			},
			expr: "response.status == 200",
			want: true,
		},
		{
			name:    "invalid expression",
			vars:    map[string]interface{}{},
			expr:    "??? invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext()
			for k, v := range tt.vars {
				ctx.Set(k, v)
			}
			got, err := ctx.EvalBool(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvalBool(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("EvalBool(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestContext_ResolveInterface(t *testing.T) {
	ctx := NewContext()
	ctx.Set("user_id", "42")
	ctx.Set("token", "abc")

	t.Run("resolves map values", func(t *testing.T) {
		input := map[string]interface{}{
			"user_id": "${user_id}",
			"name":    "literal",
		}
		got := ctx.ResolveInterface(input).(map[string]interface{})
		if got["user_id"] != "42" {
			t.Errorf("expected user_id=42, got %v", got["user_id"])
		}
		if got["name"] != "literal" {
			t.Errorf("expected name=literal, got %v", got["name"])
		}
	})

	t.Run("resolves slice values", func(t *testing.T) {
		input := []interface{}{"${token}", "fixed"}
		got := ctx.ResolveInterface(input).([]interface{})
		if got[0] != "abc" {
			t.Errorf("expected abc, got %v", got[0])
		}
		if got[1] != "fixed" {
			t.Errorf("expected fixed, got %v", got[1])
		}
	})

	t.Run("passes through non-string types", func(t *testing.T) {
		got := ctx.ResolveInterface(42)
		if got != 42 {
			t.Errorf("expected 42, got %v", got)
		}
	})
}

func TestContext_SaveFromResult(t *testing.T) {
	ctx := NewContext()

	result := map[string]interface{}{
		"response": map[string]interface{}{
			"body": map[string]interface{}{
				"token": "jwt-xyz",
				"user": map[string]interface{}{
					"id": 42,
				},
			},
		},
	}

	saveMap := map[string]string{
		"token":   "response.body.token",
		"user_id": "response.body.user.id",
	}

	if err := ctx.SaveFromResult(saveMap, result); err != nil {
		t.Fatalf("SaveFromResult error: %v", err)
	}

	token, ok := ctx.Get("token")
	if !ok || token != "jwt-xyz" {
		t.Errorf("expected token=jwt-xyz, got %v", token)
	}

	userID, ok := ctx.Get("user_id")
	if !ok || userID != 42 {
		t.Errorf("expected user_id=42, got %v", userID)
	}
}

func TestContext_ResolveInterface_DeepNested(t *testing.T) {
	ctx := NewContext()
	ctx.Set("order_id", "ORD-123")
	ctx.Set("city", "New York")
	ctx.Set("user_name", "John Doe")

	t.Run("deeply nested object with variables", func(t *testing.T) {
		input := map[string]interface{}{
			"order": map[string]interface{}{
				"id": "${order_id}",
				"customer": map[string]interface{}{
					"name": "${user_name}",
					"address": map[string]interface{}{
						"city":    "${city}",
						"country": "USA",
					},
				},
			},
		}

		got := ctx.ResolveInterface(input).(map[string]interface{})
		order := got["order"].(map[string]interface{})

		if order["id"] != "ORD-123" {
			t.Errorf("expected order.id=ORD-123, got %v", order["id"])
		}

		customer := order["customer"].(map[string]interface{})
		if customer["name"] != "John Doe" {
			t.Errorf("expected customer.name='John Doe', got %v", customer["name"])
		}

		address := customer["address"].(map[string]interface{})
		if address["city"] != "New York" {
			t.Errorf("expected address.city='New York', got %v", address["city"])
		}
		if address["country"] != "USA" {
			t.Errorf("expected address.country='USA', got %v", address["country"])
		}
	})

	t.Run("array of objects with variables", func(t *testing.T) {
		input := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"ref":  "${order_id}",
					"name": "Item 1",
				},
				map[string]interface{}{
					"ref":  "${order_id}-2",
					"name": "Item 2",
				},
			},
		}

		got := ctx.ResolveInterface(input).(map[string]interface{})
		items := got["items"].([]interface{})

		item1 := items[0].(map[string]interface{})
		if item1["ref"] != "ORD-123" {
			t.Errorf("expected items[0].ref='ORD-123', got %v", item1["ref"])
		}

		item2 := items[1].(map[string]interface{})
		if item2["ref"] != "ORD-123-2" {
			t.Errorf("expected items[1].ref='ORD-123-2', got %v", item2["ref"])
		}
	})

	t.Run("nested arrays", func(t *testing.T) {
		input := []interface{}{
			[]interface{}{
				"${order_id}",
				"fixed",
			},
			[]interface{}{
				"${city}",
				"${user_name}",
			},
		}

		got := ctx.ResolveInterface(input).([]interface{})
		inner1 := got[0].([]interface{})
		inner2 := got[1].([]interface{})

		if inner1[0] != "ORD-123" {
			t.Errorf("expected [0][0]='ORD-123', got %v", inner1[0])
		}
		if inner2[0] != "New York" {
			t.Errorf("expected [1][0]='New York', got %v", inner2[0])
		}
		if inner2[1] != "John Doe" {
			t.Errorf("expected [1][1]='John Doe', got %v", inner2[1])
		}
	})

	t.Run("mixed types preserved", func(t *testing.T) {
		input := map[string]interface{}{
			"string_val":  "${order_id}",
			"int_val":     42,
			"float_val":   3.14,
			"bool_val":    true,
			"nil_val":     nil,
			"array_mixed": []interface{}{1, "two", 3.0, true, nil},
		}

		got := ctx.ResolveInterface(input).(map[string]interface{})

		if got["string_val"] != "ORD-123" {
			t.Errorf("expected string_val='ORD-123', got %v", got["string_val"])
		}
		if got["int_val"] != 42 {
			t.Errorf("expected int_val=42, got %v", got["int_val"])
		}
		if got["float_val"] != 3.14 {
			t.Errorf("expected float_val=3.14, got %v", got["float_val"])
		}
		if got["bool_val"] != true {
			t.Errorf("expected bool_val=true, got %v", got["bool_val"])
		}
		if got["nil_val"] != nil {
			t.Errorf("expected nil_val=nil, got %v", got["nil_val"])
		}

		arr := got["array_mixed"].([]interface{})
		if arr[0] != 1 {
			t.Errorf("expected array[0]=1, got %v", arr[0])
		}
		if arr[1] != "two" {
			t.Errorf("expected array[1]='two', got %v", arr[1])
		}
	})
}

func TestContext_ResolveInterfaceTypeCoercion(t *testing.T) {
	ctx := NewContext()
	ctx.Set("is_active", true)
	ctx.Set("retry_count", 3)
	ctx.Set("price", 19.99)
	ctx.Set("meta", map[string]interface{}{"tags": []interface{}{"a", "b"}})

	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{
			name:  "exact match bool",
			input: "${is_active}",
			want:  true,
		},
		{
			name:  "exact match int",
			input: "${retry_count}",
			want:  3,
		},
		{
			name:  "exact match float",
			input: "${price}",
			want:  19.99,
		},
		{
			name:  "exact match map",
			input: "${meta}",
			want:  map[string]interface{}{"tags": []interface{}{"a", "b"}},
		},
		{
			name:  "interpolated string (no coercion)",
			input: "status is ${is_active}",
			want:  "status is true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ctx.ResolveInterface(tt.input)
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tt.want) {
				t.Errorf("ResolveInterface() = %v, want %v", got, tt.want)
			}
			if fmt.Sprintf("%T", got) != fmt.Sprintf("%T", tt.want) {
				t.Errorf("ResolveInterface() type = %T, want %T", got, tt.want)
			}
		})
	}
}

