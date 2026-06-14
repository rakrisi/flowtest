package driver

import (
	"context"
	"fmt"
	"sync"

	"github.com/radhe-singh/flowtest/internal/config"
	"github.com/radhe-singh/flowtest/internal/engine"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoDriver executes operations against a MongoDB instance.
type MongoDriver struct {
	mu     sync.Mutex
	client *mongo.Client
	name   string
	dsn    string
	dbName string
}

// NewMongoDriver creates a MongoDB driver for the given named database.
func NewMongoDriver(name, dsn string) (*MongoDriver, error) {
	dbName, err := config.ParseMongoDBName(dsn)
	if err != nil {
		return nil, err
	}
	return &MongoDriver{name: name, dsn: dsn, dbName: dbName}, nil
}

func (d *MongoDriver) Name() string { return d.name }

func (d *MongoDriver) Execute(ctx context.Context, stepConfig interface{}, flowCtx *engine.Context, env *config.EnvConfig) (map[string]interface{}, error) {
	client, err := d.getClient(ctx)
	if err != nil {
		return nil, err
	}

	db := client.Database(d.dbName)

	switch cfg := stepConfig.(type) {
	case *config.DBStepConfig:
		return d.executeOperation(ctx, db, cfg)
	default:
		return nil, fmt.Errorf("mongo driver %q: unsupported config type %T", d.name, stepConfig)
	}
}

func (d *MongoDriver) getClient(ctx context.Context) (*mongo.Client, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client != nil {
		return d.client, nil
	}

	client, err := mongo.Connect(options.Client().ApplyURI(d.dsn))
	if err != nil {
		return nil, fmt.Errorf("mongo driver %q: connecting to %s: %w", d.name, config.RedactDSN(d.dsn), err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(ctx)
		return nil, fmt.Errorf("mongo driver %q: pinging %s: %w", d.name, config.RedactDSN(d.dsn), err)
	}

	d.client = client
	return client, nil
}

func (d *MongoDriver) executeOperation(ctx context.Context, db *mongo.Database, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	if cfg.Collection == "" {
		return nil, fmt.Errorf("mongo driver %q: collection is required", d.name)
	}

	op := cfg.Operation
	if op == "" {
		op = "find"
	}

	coll := db.Collection(cfg.Collection)

	switch op {
	case "find":
		return d.opFind(ctx, coll, cfg)
	case "findOne":
		return d.opFindOne(ctx, coll, cfg)
	case "insertOne":
		return d.opInsertOne(ctx, coll, cfg)
	case "insertMany":
		return d.opInsertMany(ctx, coll, cfg)
	case "updateOne":
		return d.opUpdateOne(ctx, coll, cfg)
	case "deleteOne":
		return d.opDeleteOne(ctx, coll, cfg)
	case "deleteMany":
		return d.opDeleteMany(ctx, coll, cfg)
	case "countDocuments":
		return d.opCountDocuments(ctx, coll, cfg)
	default:
		return nil, fmt.Errorf("mongo driver %q: unsupported operation %q", d.name, op)
	}
}

func (d *MongoDriver) opFind(ctx context.Context, coll *mongo.Collection, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	filter := toBSON(cfg.Filter)

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mongo driver %q: find: %w", d.name, err)
	}
	defer cursor.Close(ctx)

	var results []interface{}
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("mongo driver %q: decoding document: %w", d.name, err)
		}
		results = append(results, normalizeDoc(doc))
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("mongo driver %q: cursor error: %w", d.name, err)
	}

	return map[string]interface{}{
		"rows":      results,
		"row_count": len(results),
	}, nil
}

func (d *MongoDriver) opFindOne(ctx context.Context, coll *mongo.Collection, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	filter := toBSON(cfg.Filter)

	var doc bson.M
	err := coll.FindOne(ctx, filter).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return map[string]interface{}{
			"rows":      []interface{}{},
			"row_count": 0,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mongo driver %q: findOne: %w", d.name, err)
	}

	return map[string]interface{}{
		"rows":      []interface{}{normalizeDoc(doc)},
		"row_count": 1,
	}, nil
}

func (d *MongoDriver) opInsertOne(ctx context.Context, coll *mongo.Collection, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	if cfg.Document == nil {
		return nil, fmt.Errorf("mongo driver %q: insertOne requires document", d.name)
	}

	result, err := coll.InsertOne(ctx, cfg.Document)
	if err != nil {
		return nil, fmt.Errorf("mongo driver %q: insertOne: %w", d.name, err)
	}

	return map[string]interface{}{
		"rows": []interface{}{
			map[string]interface{}{"_id": fmt.Sprintf("%v", result.InsertedID)},
		},
		"row_count": 1,
	}, nil
}

func (d *MongoDriver) opInsertMany(ctx context.Context, coll *mongo.Collection, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	if len(cfg.Documents) == 0 {
		return nil, fmt.Errorf("mongo driver %q: insertMany requires documents", d.name)
	}

	docs := make([]interface{}, len(cfg.Documents))
	copy(docs, cfg.Documents)

	result, err := coll.InsertMany(ctx, docs)
	if err != nil {
		return nil, fmt.Errorf("mongo driver %q: insertMany: %w", d.name, err)
	}

	var rows []interface{}
	for _, id := range result.InsertedIDs {
		rows = append(rows, map[string]interface{}{"_id": fmt.Sprintf("%v", id)})
	}

	return map[string]interface{}{
		"rows":      rows,
		"row_count": len(result.InsertedIDs),
	}, nil
}

func (d *MongoDriver) opUpdateOne(ctx context.Context, coll *mongo.Collection, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	filter := toBSON(cfg.Filter)
	update := toBSON(cfg.Update)

	// Wrap in $set if not already using an update operator
	if _, hasSet := cfg.Update["$set"]; !hasSet {
		if _, hasUnset := cfg.Update["$unset"]; !hasUnset {
			update = bson.M{"$set": update}
		}
	}

	result, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return nil, fmt.Errorf("mongo driver %q: updateOne: %w", d.name, err)
	}

	return map[string]interface{}{
		"rows":           []interface{}{},
		"row_count":      int(result.MatchedCount),
		"modified_count": int(result.ModifiedCount),
	}, nil
}

func (d *MongoDriver) opDeleteOne(ctx context.Context, coll *mongo.Collection, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	filter := toBSON(cfg.Filter)

	result, err := coll.DeleteOne(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mongo driver %q: deleteOne: %w", d.name, err)
	}

	return map[string]interface{}{
		"rows":      []interface{}{},
		"row_count": int(result.DeletedCount),
	}, nil
}

func (d *MongoDriver) opDeleteMany(ctx context.Context, coll *mongo.Collection, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	filter := toBSON(cfg.Filter)

	result, err := coll.DeleteMany(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mongo driver %q: deleteMany: %w", d.name, err)
	}

	return map[string]interface{}{
		"rows":      []interface{}{},
		"row_count": int(result.DeletedCount),
	}, nil
}

func (d *MongoDriver) opCountDocuments(ctx context.Context, coll *mongo.Collection, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	filter := toBSON(cfg.Filter)

	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mongo driver %q: countDocuments: %w", d.name, err)
	}

	return map[string]interface{}{
		"rows":      []interface{}{},
		"row_count": int(count),
	}, nil
}

// Close disconnects the MongoDB client.
func (d *MongoDriver) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.client != nil {
		d.client.Disconnect(context.Background())
		d.client = nil
	}
}

// toBSON converts a map[string]interface{} to bson.M for use in queries.
func toBSON(m map[string]interface{}) bson.M {
	if m == nil {
		return bson.M{}
	}
	result := bson.M{}
	for k, v := range m {
		result[k] = v
	}
	return result
}

// normalizeDoc converts a bson.M to map[string]interface{} for uniform result handling.
func normalizeDoc(doc bson.M) map[string]interface{} {
	result := make(map[string]interface{}, len(doc))
	for k, v := range doc {
		switch val := v.(type) {
		case bson.M:
			result[k] = normalizeDoc(val)
		case bson.A:
			arr := make([]interface{}, len(val))
			for i, elem := range val {
				if m, ok := elem.(bson.M); ok {
					arr[i] = normalizeDoc(m)
				} else {
					arr[i] = elem
				}
			}
			result[k] = arr
		default:
			result[k] = v
		}
	}
	return result
}
