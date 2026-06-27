package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var ErrClusterExists = errors.New("cluster already exists")
var ErrClusterNotFound = errors.New("cluster not found")
var ErrWorkerExists = errors.New("worker already exists")
var ErrWorkerNotFound = errors.New("worker not found")

type Store struct {
	ddb   *dynamodb.Client
	table string
}

func New(ddb *dynamodb.Client, table string) *Store {
	return &Store{ddb: ddb, table: table}
}

// created is true when the table didn't exist and EnsureTable had to make
// it, so callers can warn that it happened implicitly.
func (s *Store) EnsureTable(ctx context.Context) (created bool, err error) {
	_, err = s.ddb.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(s.table)})
	if err == nil {
		return false, nil
	}

	var nfe *types.ResourceNotFoundException
	if !errors.As(err, &nfe) {
		return false, fmt.Errorf("describe table %q: %w", s.table, err)
	}

	_, err = s.ddb.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(s.table),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("name"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("name"), KeyType: types.KeyTypeHash},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		return false, fmt.Errorf("create table %q: %w", s.table, err)
	}

	waiter := dynamodb.NewTableExistsWaiter(s.ddb)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(s.table)}, 2*time.Minute); err != nil {
		return false, fmt.Errorf("wait for table %q: %w", s.table, err)
	}

	return true, nil
}

func (s *Store) GetCluster(ctx context.Context, name string) (*Cluster, error) {
	out, err := s.ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"name": &types.AttributeValueMemberS{Value: name},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("get cluster %q: %w", name, err)
	}

	if out.Item == nil {
		return nil, ErrClusterNotFound
	}

	var c Cluster
	if err := attributevalue.UnmarshalMap(out.Item, &c); err != nil {
		return nil, fmt.Errorf("unmarshal cluster %q: %w", name, err)
	}

	return &c, nil
}

func (s *Store) ListClusters(ctx context.Context) ([]Cluster, error) {
	out, err := s.ddb.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(s.table)})
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}

	clusters := make([]Cluster, 0, len(out.Items))
	for _, item := range out.Items {
		var c Cluster
		if err := attributevalue.UnmarshalMap(item, &c); err != nil {
			return nil, fmt.Errorf("unmarshal cluster: %w", err)
		}
		clusters = append(clusters, c)
	}

	return clusters, nil
}

func (s *Store) PutNewCluster(ctx context.Context, c Cluster) error {
	item, err := attributevalue.MarshalMap(c)
	if err != nil {
		return fmt.Errorf("marshal cluster %q: %w", c.Name, err)
	}

	_, err = s.ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(#n)"),
		ExpressionAttributeNames: map[string]string{
			"#n": "name",
		},
	})
	if err != nil {
		var cce *types.ConditionalCheckFailedException
		if errors.As(err, &cce) {
			return ErrClusterExists
		}
		return fmt.Errorf("put cluster %q: %w", c.Name, err)
	}

	return nil
}

func (s *Store) SaveCluster(ctx context.Context, c Cluster) error {
	item, err := attributevalue.MarshalMap(c)
	if err != nil {
		return fmt.Errorf("marshal cluster %q: %w", c.Name, err)
	}

	_, err = s.ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("save cluster %q: %w", c.Name, err)
	}

	return nil
}

func (s *Store) DeleteCluster(ctx context.Context, name string) error {
	_, err := s.ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"name": &types.AttributeValueMemberS{Value: name},
		},
		ConditionExpression: aws.String("attribute_exists(#n)"),
		ExpressionAttributeNames: map[string]string{
			"#n": "name",
		},
	})
	if err != nil {
		var cce *types.ConditionalCheckFailedException
		if errors.As(err, &cce) {
			return ErrClusterNotFound
		}
		return fmt.Errorf("delete cluster %q: %w", name, err)
	}

	return nil
}
