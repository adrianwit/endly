package dsunit

import (
	"fmt"
	"github.com/viant/dsunit"
	"github.com/viant/endly"
	"github.com/viant/toolbox"
	"github.com/viant/toolbox/data"
)

var converter = toolbox.NewColumnConverter("yyyy-MM-dd HH:ss")

const (
	//ServiceID represents a data store unit service id
	ServiceID = "dsunit"
)

//PopulateDatastoreEvent represents a populate Datastore event
type PopulateDatastoreEvent struct {
	Datastore string `required:"true" description:"register datastore name"` //target host
	Table     string
	Rows      int
}

//RunSQLcriptEvent represents run script event
type RunSQLcriptEvent struct {
	Datastore string
	URL       string
}

type service struct {
	*endly.AbstractService
	Service dsunit.Service
}

const (
	dsunitMySQLInitExample = `{
  "Datastore": "mydb",
  "Config": {
    "DriverName": "mysql",
    "Descriptor": "[username]:[password]@tcp(127.0.0.1:3306)/[dbname]?parseTime=true",
    "Credentials": "$mysqlCredentials"
  },
  "Admin": {
    "Datastore": "mysql",
    "Config": {
      "DriverName": "mysql",
      "Descriptor": "[username]:[password]@tcp(127.0.0.1:3306)/[dbname]?parseTime=true",
      "Credentials": "$mysqlCredentials"
    }
  },
  "Scripts": [
    {
      "URL": "datastore/mydb/schema.ddl"
    }
  ],
  "Recreate": "true"
}`

	dsunitAerospikeRegisterExample = `{
  "Datastore": "db",
  "Config": {
    "DriverName": "aerospike",
    "Descriptor": "tcp([host]:3000)/[namespace]",
    "Parameters": {
      "dateFormat": "yyyy-MM-dd hh:mm:ss",
      "dbname": "db",
      "host": "127.0.0.1",
      "keyColumnName": "id",
      "namespace": "db",
      "port": "3000"
    }
  }
}`

	dsunitBigQueryRegisterExample = `{
  "Datastore": "db1",
  "Config": {
    "DriverName": "bigquery",
    "Descriptor": "bq/[datasetId]",
	"Credentials": "${env.HOME}/.secret/bq.json",
    "Parameters": {
      "datasetId": "db1",
      "dateFormat": "yyyy-MM-dd HH:mm:ss.SSSZ",
      "projectId": "xxxxx"
    }
  }
	
}`

	dsunitMySQLRegisterExample = `{
  "Datastore": "db1",
  "Config": {
    "DriverName": "mysql",
  	"Credentials": "${env.HOME}/.secret/mysql.json",
    "Descriptor": "[username]:[password]@tcp(127.0.0.1:3306)/[dbname]?parseTime=true"
  }
}`

	dsunitServiceSQLExample = `{
		"Datastore": "db1",
		"Scripts": [
			{
				"URL": "datastore/db1/schema.ddl"
			}
		]
	}`

	dsunitServiceMappingExample = ` {
		"Mappings": [
			{
				"URL": "config/mapping/v_asset.json"
			}
		]
	}`

	dsunitServiceSequenceExample = `{
		"Datastore": "db1",
		"Tables": [
			"table1",
			"table2"
		]
	}`

	dsunitServiceStaticDataPrepareExample = `{
    "Datastore": "db1",
    "URL": "datastore/db1/dictionary"
  }`

	dsunitDataPrepareExaple = ` {
		"Datastore": "db1",
		"Data": {
			"table1": [
				{
					"id": 1,
					"name": "test 1",
					"type": "pivot"
				},
				{
					"id": 2,
					"name": "test 2",
					"type": "pivot"
				}
			],
			"table2": [
				{
					"id": 1,
					"name": "test 1",
					"type": "pivot"
				},
				{
					"id": 2,
					"name": "test 2",
					"type": "pivot"
				}
			]
		}
	}`

	dsunitServiceExpectAction = `{
    "Datastore": "db1",
    "URL": "datastore/db1/use_case2/",
	"Prefix":"expect_"
  }`
)

func (s *service) registerRoutes() {

	s.Register(&endly.Route{
		Action: "register",
		RequestInfo: &endly.ActionInfo{
			Description: "register database connection",
			Examples: []*endly.UseCase{
				{
					Description: "aerospike datastore registration",
					Data:        dsunitAerospikeRegisterExample,
				},
				{
					Description: "BigQuery datastore registration",
					Data:        dsunitBigQueryRegisterExample,
				},

				{
					Description: "MySQL datastore registration",
					Data:        dsunitMySQLRegisterExample,
				},
			},
		},
		RequestProvider: func() interface{} {
			return &RegisterRequest{}
		},
		ResponseProvider: func() interface{} {
			return &RegisterResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {

			if req, ok := request.(*RegisterRequest); ok {
				var dsRequest = dsunit.RegisterRequest(*req)
				request = &dsRequest
			}

			if req, ok := request.(*dsunit.RegisterRequest); ok {
				resp := s.Service.Register(req)
				response := RegisterResponse(*resp)
				return &response, response.Error()
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})

	s.Register(&endly.Route{
		Action: "recreate",
		RequestInfo: &endly.ActionInfo{
			Description: "create datastore",
			Examples:    []*endly.UseCase{},
		},
		RequestProvider: func() interface{} {
			return &RecreateRequest{}
		},
		ResponseProvider: func() interface{} {
			return &RecreateResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {
			if req, ok := request.(*RecreateRequest); ok {
				var dsRequest = dsunit.RecreateRequest(*req)
				request = &dsRequest
			}

			if req, ok := request.(*dsunit.RecreateRequest); ok {
				resp := s.Service.Recreate(req)
				response := RecreateResponse(*resp)
				return &response, response.Error()
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})

	s.Register(&endly.Route{
		Action: "script",
		RequestInfo: &endly.ActionInfo{
			Description: "run SQL script",
			Examples: []*endly.UseCase{
				{
					Description: "run script",
					Data:        dsunitServiceSQLExample,
				},
			},
		},
		RequestProvider: func() interface{} {
			return &RunScriptRequest{}
		},
		ResponseProvider: func() interface{} {
			return &RunSQLResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {
			if req, ok := request.(*RunScriptRequest); ok {
				var dsRequest = dsunit.RunScriptRequest(*req)
				request = &dsRequest
			}
			if req, ok := request.(*dsunit.RunScriptRequest); ok {
				resp := s.Service.RunScript(req)
				response := RunSQLResponse(*resp)
				return &response, response.Error()
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})

	s.Register(&endly.Route{
		Action: "sql",
		RequestInfo: &endly.ActionInfo{
			Description: "run SQL",
			Examples:    []*endly.UseCase{},
		},
		RequestProvider: func() interface{} {
			return &RunSQLRequest{}
		},
		ResponseProvider: func() interface{} {
			return &RunSQLResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {

			if req, ok := request.(*RunSQLRequest); ok {
				var dsRequest = dsunit.RunSQLRequest(*req)
				request = &dsRequest
			}

			if req, ok := request.(*dsunit.RunSQLRequest); ok {
				resp := s.Service.RunSQL(req)
				response := RunSQLResponse(*resp)
				return &response, response.Error()
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})

	s.Register(&endly.Route{
		Action: "mapping",
		RequestInfo: &endly.ActionInfo{
			Description: "register database table mapping (view)",
			Examples:    []*endly.UseCase{},
		},
		RequestProvider: func() interface{} {
			return &MappingRequest{}
		},
		ResponseProvider: func() interface{} {
			return &MappingResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {
			if req, ok := request.(*MappingRequest); ok {
				var dsRequest = dsunit.MappingRequest(*req)
				request = &dsRequest
			}
			if req, ok := request.(*dsunit.MappingRequest); ok {
				resp := s.Service.AddTableMapping(req)
				response := MappingResponse(*resp)
				return &response, response.Error()
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})

	s.Register(&endly.Route{
		Action: "init",
		RequestInfo: &endly.ActionInfo{
			Description: "initialize datastore (register, recreated, run sql, add mapping)",

			Examples: []*endly.UseCase{
				{
					Description: "mysql init",
					Data:        dsunitMySQLInitExample,
				},
			},
		},
		RequestProvider: func() interface{} {
			return &InitRequest{}
		},
		ResponseProvider: func() interface{} {
			return &InitResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {
			if req, ok := request.(*InitRequest); ok {
				var dsRequest = dsunit.InitRequest(*req)
				request = &dsRequest
			}
			if req, ok := request.(*dsunit.InitRequest); ok {
				resp := s.Service.Init(req)
				response := InitResponse(*resp)
				return &response, response.Error()
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})

	s.Register(&endly.Route{
		Action: "prepare",
		RequestInfo: &endly.ActionInfo{
			Description: "populate databstore with provided data",
			Examples: []*endly.UseCase{
				{
					Description: "static data prepare",
					Data:        dsunitServiceStaticDataPrepareExample,
				},
				{
					Description: "data prepare",
					Data:        dsunitDataPrepareExaple,
				},
			},
		},
		RequestProvider: func() interface{} {
			return &PrepareRequest{}
		},
		ResponseProvider: func() interface{} {
			return &PrepareResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {
			if req, ok := request.(*PrepareRequest); ok {
				var dsRequest = dsunit.PrepareRequest(*req)
				request = &dsRequest
			}

			if req, ok := request.(*dsunit.PrepareRequest); ok {
				resp := s.Service.Prepare(req)
				response := PrepareResponse(*resp)
				return &response, response.Error()
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})

	s.Register(&endly.Route{
		Action: "expect",
		RequestInfo: &endly.ActionInfo{
			Description: "verify databstore with provided data",
			Examples: []*endly.UseCase{
				{
					Description: "static data expect",
					Data:        dsunitServiceExpectAction,
				},
				{
					Description: "data expect",
					Data:        dsunitDataPrepareExaple,
				},
			},
		},
		RequestProvider: func() interface{} {
			return &dsunit.ExpectRequest{
				DatasetResource: &dsunit.DatasetResource{
					DatastoreDatasets: &dsunit.DatastoreDatasets{},
				},
			}
		},
		ResponseProvider: func() interface{} {
			return &dsunit.ExpectResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {
			if req, ok := request.(*ExpectRequest); ok {
				var dsRequest = dsunit.ExpectRequest(*req)
				request = &dsRequest
			}

			if req, ok := request.(*dsunit.ExpectRequest); ok {
				resp := s.Service.Expect(req)
				response := ExpectResponse(*resp)
				return &response, response.Error()
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})

	s.Register(&endly.Route{
		Action: "query",
		RequestInfo: &endly.ActionInfo{
			Description: "run SQL query",
		},
		RequestProvider: func() interface{} {
			return &QueryRequest{}
		},
		ResponseProvider: func() interface{} {
			return &QueryResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {
			if req, ok := request.(*QueryRequest); ok {
				var dsRequest = dsunit.QueryRequest(*req)
				request = &dsRequest
			}
			if req, ok := request.(*dsunit.QueryRequest); ok {
				resp := s.Service.Query(req)
				response := QueryResponse(*resp)
				var err = response.Error()
				if req.IgnoreError {
					err = nil
				}
				return &response, err
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})

	s.Register(&endly.Route{
		Action: "sequence",
		RequestInfo: &endly.ActionInfo{
			Description: "get sequence for supplied tables",
			Examples: []*endly.UseCase{
				{
					Description: "sequence",
					Data:        dsunitServiceSequenceExample,
				},
			},
		},
		RequestProvider: func() interface{} {
			return &SequenceRequest{}
		},
		ResponseProvider: func() interface{} {
			return &SequenceResponse{}
		},
		Handler: func(context *endly.Context, request interface{}) (interface{}, error) {
			if req, ok := request.(*SequenceRequest); ok {
				var dsRequest = dsunit.SequenceRequest(*req)
				request = &dsRequest
			}
			if req, ok := request.(*dsunit.SequenceRequest); ok {
				resp := s.Service.Sequence(req)
				response := SequenceResponse(*resp)
				return &response, response.Error()
			}
			return nil, fmt.Errorf("unsupported request type: %T", request)
		},
	})
}

func (s service) Run(context *endly.Context, request interface{}) *endly.ServiceResponse {
	var state = context.State()
	context.Context.Replace((*data.Map)(nil), &state)
	s.Service.SetContext(context.Context)
	return s.AbstractService.Run(context, request)
}

//New creates a new Datastore unit service
func New() endly.Service {
	var result = &service{
		AbstractService: endly.NewAbstractService(ServiceID),
		Service:         dsunit.New(),
	}
	result.AbstractService.Service = result
	result.registerRoutes()
	return result
}
