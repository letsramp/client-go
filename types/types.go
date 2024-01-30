/*
 * Copyright Skyramp Authors 2024
 */
package types

import (
	"sync"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

type UserCredential struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

// Service represents K8s SVC, FQDN, or IP
type Service struct {
	// Identifier for this service
	Name string `json:"name" yaml:"name"`
	// Port number to access this service
	Port int `json:"port,omitempty" yaml:"port,omitempty"`
	// Addr could be any form of SVC name, FQDN or IP
	Addr string `json:"addr,omitempty" yaml:"addr,omitempty"`
	// if alias is provided, Worker will update the K8s SVC / docker container to point to worker
	ServiceAlias string `json:"alias,omitempty" yaml:"alias,omitempty"`
	// If encryption is required
	Secure bool `json:"secure,omitempty" yaml:"secure,omitempty"`
	// Protocol that is associated with this service, namely gRPC, REST, Thrift or Mongo
	Protocol ProtocolType `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	// Endpoints that belong to this service
	Endpoints []string `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	// Container that this service belongs to
	Container string `json:"container,omitempty" yaml:"container,omitempty"`
	// User provided credentials
	Credential *UserCredential `json:"credential,omitempty" yaml:"credential,omitempty"`
}

type ParamType string

const (
	IntParam    ParamType = "integer"
	FloatParam  ParamType = "float"
	StringParam ParamType = "string"
	BoolParam   ParamType = "bool"
)

type RestParamKind string

const (
	PathParam  RestParamKind = "path"
	QueryParam RestParamKind = "query"
	FormParam  RestParamKind = "form"
)

// Wrapper for rest parameters
type RestParam struct {
	Name  string        `json:"name,omitempty" yaml:"name,omitempty"`
	In    RestParamKind `json:"in,omitempty" yaml:"in,omitempty"`
	Type  ParamType     `json:"type,omitempty" yaml:"type,omitempty"`
	Value interface{}   `json:"value,omitempty" yaml:"value,omitempty"`
}

type RestMethodType string

const (
	GetMethod    RestMethodType = "GET"
	PutMethod    RestMethodType = "PUT"
	PostMethod   RestMethodType = "POST"
	DeleteMethod RestMethodType = "DELETE"
	PatchMethod  RestMethodType = "PATCH"
)

const (
	ReadMethod   = "read"
	WriteMethod  = "write"
	UpdateMethod = "update"
)

type Method struct {
	// Identifier of this method
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// For REST, this specifies what type of CRUD this method is
	Type RestMethodType `json:"type,omitempty" yaml:"type,omitempty"` // REST CRUD
}

type ProtocolType string

const (
	Rest        ProtocolType = "rest"
	Grpc        ProtocolType = "grpc"
	Thrift      ProtocolType = "thrift"
	Mongo       ProtocolType = "mongo"
	JsonrpcWS   ProtocolType = "jsonrpc-ws"
	JsonrpcHttp ProtocolType = "jsonrpc-http"
)

// Wrapper for gRPC service, REST path
type Endpoint struct {
	// Identifier of this endpoint
	Name string `json:"name" yaml:"name"`
	// Methods that are associated with this endpoint, i.e., gRPC method, REST CRUD
	Methods []*Method `json:"methods,omitempty" yaml:"methods,omitempty"`
	// For REST, this is the http path
	RestPath string `json:"path,omitempty" yaml:"path,omitempty"`
	// For gRPC - this is where the original .proto lives
	Defined *DefinitionInfo `json:"defined,omitempty" yaml:"defined,omitempty"`
	// Service that this endpoint belongs to.
	ServiceName string `json:"serviceName,omitempty" yaml:"serviceName,omitempty"`
}

// For gRPC, this is used to indicate where the endpoint is defined
type DefinitionInfo struct {
	// File path that this signature is defined
	Path string `json:"path" yaml:"path"`
	// Name of gRPC service function name that this endpoint is mapped to
	// If endpoint's name is equal to what is defined, but can be omitted
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

func (d1 *DefinitionInfo) Equals(d2 *DefinitionInfo) bool {
	if d1 == nil && d2 == nil {
		return true
	} else if d1 == nil || d2 == nil {
		return false
	}

	return d1.Path == d2.Path && d1.Name == d2.Name
}

// repeat config for scenario steps
type TestStepRepeatConfig struct {
	// Number of repeatition. This cannot be used with "until"
	Count *int `json:"count,omitempty" yaml:"count,omitempty"`
	// "Until" dictates when this repeat terminates
	Until string `json:"until,omitempty" yaml:"until,omitempty"`

	UntilPython string `json:"untilPython,omitempty" yaml:"untilPython,omitempty"`
	// interval between repeated requests
	Interval interface{} `json:"interval,omitempty" yaml:"interval,omitempty"`
	// max retries dictates how many times to do this request if it does not meet "until'
	MaxRetries *int `json:"maxRetries,omitempty" yaml:"maxRetries,omitempty"`
}

type TestStep struct {
	// API requests (REST, gRPC) or db read/write
	RequestName string `json:"requestName,omitempty" yaml:"requestName,omitempty"`
	// Shell commands
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
	// Validation
	Assert       string `json:"asserts,omitempty" yaml:"asserts,omitempty"`
	AssertPython string `json:"assertsPython,omitempty" yaml:"assertsPython,omitempty"`

	// nested scenario
	ScenarioName string `json:"scenarioName,omitempty" yaml:"scenarioName,omitempty"`

	Wait interface{} `json:"wait,omitempty" yaml:"wait,omitempty"`
	// if set true, proceed to the next step even if the step failed
	Ignore bool `json:"ignore,omitempty" yaml:"ignore,omitempty"`

	// if set to true, break from here
	Break bool `json:"break,omitempty" yaml:"break,omitempty"`
	// if set to true, skip the step
	Skip bool `json:"skip,omitempty" yaml:"skip,omitempty"`

	// chaining happens with override
	Override map[string]string `json:"override,omitempty" yaml:"override,omitempty"`
	// values to export: this will be available as scenarioVars
	Export map[string]string `json:"export,omitempty" yaml:"export,omitempty"`
	// repeat
	Repeat *TestStepRepeatConfig `json:"repeat,omitempty" yaml:"repeat,omitempty"`

	// blob override
	BlobOverride map[string]interface{} `json:"blobOverride,omitempty" yaml:"blobOverride,omitempty"`
}

type Value struct {
	// For rest, these will be set to http headers
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	// For rest, path, query, form params can be defined
	Params []*RestParam `json:"params,omitempty" yaml:"params,omitempty"`
	// Json blob that describes value
	Blob string `json:"blob,omitempty" yaml:"blob,omitempty"`
	// Raw javascript
	Javascript string `json:"javascript,omitempty" yaml:"javascript,omitempty"`
	// Javascript path - it should be relative to CWD or Project root, or absolute
	JavascriptPath string `json:"javascriptPath,omitempty" yaml:"javascriptPath,omitempty"`
	// Raw python script
	Python string `json:"python,omitempty" yaml:"python,omitempty"`
	// Python script path - it should be relative to CWD or Project root, or absolute
	PythonPath string `json:"pythonPath,omitempty" yaml:"pythonPath,omitempty"`
	// primitive value for thrift
	Value interface{} `json:"value,omitempty" yaml:"value,omitempty"`
	// yamlfile
	YamlPath string `json:"yamlPath,omitempty" yaml:"yamlPath,omitempty"`
	// jsonfile
	JsonPath string `json:"jsonPath,omitempty" yaml:"jsonPath,omitempty"`
	// blob override jsonPath -> value
	BlobOverride map[string]interface{} `json:"blobOverride,omitempty" yaml:"blobOverride,omitempty"`
	// cookies
	Cookies map[string]string `json:"cookies,omitempty" yaml:"cookies,omitempty"`
}

// Response for mock service
type Response struct {
	Name   string `json:"name" yaml:"name"`
	*Value `json:",inline" yaml:",inline"`
	// For tests without container description we need to know dst port at least
	ServiceName string `json:"serviceName,omitempty" yaml:"serviceName,omitempty"`
	// endpoint name
	EndpointName string `json:"endpointName,omitempty" yaml:"endpointName,omitempty"`
	// method name
	MethodName string `json:"methodName" yaml:"methodName"`
}

type ResponseConfig struct {
	// Identifier of this value
	ResponseName string `json:"responseName,omitempty" yaml:"responseName,omitempty"`
	// Configuration for traffic
	*TrafficConfig `json:",inline" yaml:",inline"`
}

// Configurations for delayed traffic
type TrafficDelayConfig struct {
	// Minimum amount of delay, in ms
	MinDelay int `json:"minDelay,omitempty" yaml:"minDelay,omitempty"`
	// Maximum amount of delay, in ms
	MaxDelay int `json:"maxDelay,omitempty" yaml:"maxDelay,omitempty"`
}

// Contains configurations for traffic (request/reply)
type TrafficConfig struct {
	// Configuration for delays applied to this traffic configuration
	DelayConfig *TrafficDelayConfig `json:"delayConfig,omitempty" yaml:"delayConfig,omitempty"`
	// Represents a number in the range [0,100] for the percentage of requests that will error out
	LossPercentage int `json:"lossPercentage,omitempty" yaml:"lossPercentage,omitempty"`
}

type TestRequest struct {
	Name   string `json:"name" yaml:"name"`
	*Value `json:",inline" yaml:",inline"`
	// For tests without container description we need to know dst port at least
	ServiceName string `json:"serviceName,omitempty" yaml:"serviceName,omitempty"`
	// endpoint name
	EndpointName string `json:"endpointName,omitempty" yaml:"endpointName,omitempty"`
	// method name
	MethodName string   `json:"methodName" yaml:"methodName"`
	Asserts    []string `json:"asserts,omitempty" yaml:"asserts,omitempty"`
	// DB related
	DBParam *DBParam `json:"dbParam,omitempty" yaml:"dbParam,omitempty"`
	// Mongo Read options
	MongoReadParam *MongoReadParam `json:"mongoReadParam,omitempty" yaml:"mongoReadParam,omitempty"`
	// Mongo Update options
	MongoUpdateParam *MongoUpdateParam      `json:"mongoUpdateParam,omitempty" yaml:"mongoUpdateParam,omitempty"`
	Vars             map[string]interface{} `json:"vars,omitempty" yaml:"vars,omitempty"`
	// Response properties (for POST only) to use in CRUD scenario path chaining
	ResponseProps map[string]*openapi3.SchemaRef `json:"-" yaml:"-"`
}

type TestCommand struct {
	Name      string   `json:"name" yaml:"name"`
	Command   string   `json:"command" yaml:"command"`
	Dir       string   `json:"dir,omitempty" yaml:"dir,omitempty"`
	Container string   `json:"container,omitempty" yaml:"container,omitempty"`
	Asserts   []string `json:"asserts,omitempty" yaml:"asserts,omitempty"`

	Vars map[string]interface{} `json:"vars,omitempty" yaml:"vars,omitempty"`
}

type TestType string

const (
	TestTypeLoad       TestType = "load"
	TestTypeFunctional TestType = "functional"
	TestTypeStress     TestType = "stress"
)

type TestScenario struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// scenario level vars - mutable
	Vars map[string]interface{} `json:"vars,omitempty" yaml:"vars,omitempty"`
	// scenario level headers
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	// scenario level cookies
	Cookies map[string]string `json:"cookies,omitempty" yaml:"cookies,omitempty"`
	// For iteration
	With   []map[string]interface{} `json:"with,omitempty" yaml:"with,omitempty"`
	Steps  []*TestStep              `json:"steps" yaml:"steps"`
	Ignore bool                     `json:"ignore,omitempty" yaml:"ignore,omitempty"`
}

type TestRampConfig struct {
	Interval interface{} `json:"interval" yaml:"interval"`
	Duration interface{} `json:"duration" yaml:"duration"`
}

type TestPattern struct {
	// Scenario
	ScenarioName string `json:"scenarioName,omitempty" yaml:"scenarioName,omitempty"`
	// Shell commands
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
	// API requests (REST, gRPC, Thrift) or db read / write
	RequestName string `json:"requestName,omitempty" yaml:"requestName,omitempty"`

	Start    interface{}     `json:"startAt,omitempty" yaml:"startAt,omitempty"`
	Duration interface{}     `json:"duration,omitempty" yaml:"duration,omitempty"`
	AtOnce   int             `json:"atOnce,omitempty" yaml:"atOnce,omitempty"`
	RampUp   *TestRampConfig `json:"rampUp,omitempty" yaml:"rampUp,omitempty"`
	Steps    []*TestPattern  `json:"steps,omitempty" yaml:"steps,omitempty"`
	Timeout  interface{}     `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Asserts  []string        `json:"asserts,omitempty" yaml:"asserts,omitempty"`
	// RPS target
	TargetRPS int `json:"targetRPS,omitempty" yaml:"targetRPS,omitempty"`
}

type PreExecActionType string

const (
	PreExecCopy PreExecActionType = "copy"
	PreExecRun  PreExecActionType = "run"
)

type PostExecActionType string

const (
	PostExecFetchAndRun PostExecActionType = "fetchAndRun"
)

type ExecConfig struct {
	Namespace string   `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Container string   `json:"container,omitempty" yaml:"container,omitempty"`
	Dir       string   `json:"dir,omitempty" yaml:"dir,omitempty"`
	Command   string   `json:"command,omitempty" yaml:"command,omitempty"`
	Args      []string `json:"args,omitempty" yaml:"args,omitempty"`
	Envs      []string `json:"envs,omitempty" yaml:"envs,omitempty"`
}

type PreExecConfig struct {
	ExecConfig `json:",inline" yaml:",inline"`

	Action           PreExecActionType `json:"action" yaml:"action"`
	ContainerDstPath string            `json:"containerDstPath,omitempty" yaml:"containerDstPath,omitempty"`
	LocalSrcPath     string            `json:"localSrcPath,omitempty" yaml:"localSrcPath,omitempty"`
}

type PostExecConfig struct {
	ExecConfig `json:",inline" yaml:",inline"`

	Action           PostExecActionType `json:"action" yaml:"action"`
	ContainerSrcPath string             `json:"containerSrcPath,omitempty" yaml:"containerSrcPath,omitempty"`
	LocalDstPath     string             `json:"localDstPath,omitempty" yaml:"localDstPath,omitempty"`
}

// Test
type Test struct {
	Name        string         `json:"name,omitempty" yaml:"name,omitempty"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Patterns    []*TestPattern `json:"testPattern" yaml:"testPattern"`
	InputFile   string         `json:"inputFile,omitempty" yaml:"inputFile,omitempty"`
	// target to run test in k8s backend
	Target     string      `json:"target,omitempty" yaml:"target,omitempty"`
	Container  string      `json:"container,omitempty" yaml:"container,omitempty"`
	Timeout    interface{} `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	OutputFile string      `json:"outputFile,omitempty" yaml:"outputFile,omitempty"`
	// Any actions to be performed after executing test
	PostExecutions []*PostExecConfig `json:"postExecution,omitempty" yaml:"postExecution,omitempty"`
	// Any actions to be performed before executing test
	PreExecutions []*PreExecConfig `json:"preExecution,omitempty" yaml:"preExecution,omitempty"`
	Override      *TestOverride    `json:"override,omitempty" yaml:"override,omitempty"`

	// global variables. Immutable
	GlobalVars map[string]interface{} `json:"globalVars,omitempty" yaml:"globalVars,omitempty"`
	// global headers Immutable
	GlobalHeaders map[string]string `json:"globalHeaders,omitempty" yaml:"globalHeaders,omitempty"`

	// npm packages
	NpmPackages []string `json:"npmPackages,omitempty" yaml:"npmPackages,omitempty"`

	// pip requirement path
	RequirementPath string `json:"pipRequirements,omitempty" yaml:"pipRequirements,omitempty"`
}

type TestResults map[int]*TestResultNode

type TestResultType string

const (
	TestResultNone     TestResultType = ""
	TestResultCommand  TestResultType = "command"
	TestResultRequest  TestResultType = "request"
	TestResultScenario TestResultType = "scenario"
	TestResultLoad     TestResultType = "load"
	TestResultRepeat   TestResultType = "repeat"
	TestResultAssert   TestResultType = "assert"
	TestResultWith     TestResultType = "with"
)

type TestTimeseriesStat struct {
	Timestamp  time.Duration `json:"timestamp" yaml:"timestamp" bson:"timestamp"`
	ErrorRate  float64       `json:"errorRate" yaml:"errorRate" bson:"errorRate"`
	RPS        int64         `json:"RPS" yaml:"RPS" bson:"RPS"`
	AvgLatency time.Duration `json:"avgLatency" yaml:"avgLatency" bson:"avgLatency"`
}

type TestResult struct {
	Status      TesterStatusType `json:"status,omitempty" yaml:"status,omitempty" bson:"status,omitempty"`
	Description string           `json:"description,omitempty" yaml:"description,omitempty" bson:"description,omitempty"`
	NestedInfo  string           `json:"nestedInfo" yaml:"nestedInfo" bson:"nestedInfo"`
	// Whether request was executed without errors
	Executed bool   `json:"executed,omitempty" yaml:"executed,omitempty" bson:"executed,omitempty"`
	Error    string `json:"error,omitempty" yaml:"error,omitempty" bson:"error,omitempty"`
	// For saving input, for output validation in future
	Input string `json:"input,omitempty" yaml:"input,omitempty" bson:"input,omitempty"`
	// Output of command / request
	Output string `json:"output,omitempty" yaml:"output,omitempty" bson:"output,omitempty"`

	Type TestResultType `json:"type,omitempty" yaml:"type,omitempty" bson:"type,omitempty"`

	Begin    time.Time     `json:"begin,omitempty" yaml:"begin,omitempty"`
	End      time.Time     `json:"end,omitempty" yaml:"end,omitempty"`
	Duration time.Duration `json:"duration,omitempty" yaml:"duration,omitempty"`

	// for load testing, we expose list of instantaneous stats as well
	Timeseries []*TestTimeseriesStat `json:"timeseries,omitempty" yaml:"timeseries,omitempty"`

	Stat map[string]*TestStat `json:"stat,omitempty" yaml:"stat,omitempty"`
}

type TestResultNode struct {
	TestResult

	// scenario / pattern steps
	StepResults TestResults `json:"stepResults,omitempty" yaml:"stepResults,omitempty"`
	// For Load testing for goroutines
	LoadResults TestResults `json:"loadResults,omitempty" yaml:"loadResults,omitempty"`
	// For repeated testing
	RepeatResults TestResults `json:"repeatResults,omitempty" yaml:"repeatResults,omitempty"`

	// Maybe we could use RWMutex
	Mutex sync.Mutex `json:"-" yaml:"-"`
}

// For communication between binary and worker over http
type TestPostRequest struct {
	Description *TestDescription  `json:"description" yaml:"description"`
	MockDesc    *MockDescription  `json:"mockDescription,omitempty" yaml:"mockDescription,omitempty"`
	Files       map[string][]byte `json:"files,omitempty" yaml:"files,omitempty"`
	Javascripts map[string][]byte `json:"javascripts,omitempty" yaml:"javascripts,omitempty"`
}

type TesterStatusType string

const (
	TesterIdle         TesterStatusType = "idle"
	TesterInitializing TesterStatusType = "initializing"
	TesterWaiting      TesterStatusType = "waiting"
	TesterRunning      TesterStatusType = "running"
	TesterFailed       TesterStatusType = "failed"
	TesterSkipped      TesterStatusType = "skipped"
	TesterStopping     TesterStatusType = "stopping"
	TesterStopped      TesterStatusType = "stopped" // user stopped
	TesterFinished     TesterStatusType = "finished"
)

type TestStatusResponse struct {
	Status    TesterStatusType     `json:"status,omitempty" yaml:"status,omitempty" bson:"status,omitempty"`
	Error     string               `json:"error,omitempty" yaml:"error,omitempty" bson:"error,omitempty"`
	Message   string               `json:"message,omitempty" yaml:"message,omitempty" bson:"message,omitempty"`
	TestName  string               `json:"test,omitempty" yaml:"test,omitempty" bson:"test,omitempty"`
	Results   []*TestResult        `json:"results,omitempty" yaml:"results,omitempty" bson:"results,omitempty"`
	Stats     map[string]*TestStat `json:"stats,omitempty" yaml:"stats,omitempty" bson:"stats,omitempty"`
	Timestamp time.Time            `json:"timestamp" yaml:"timestamp" bson:"timestamp"`
	TestID    string               `json:"id,omitempty" yaml:"id,omitempty" bson:"id,omitempty"`
	// System Metrics collected during load test
	// ContainerStats map[string]*k8sMetrics.ContainerStat `json:"containerStats,omitempty" yaml:"containerStats,omitempty" bson:"containerStats,omitempty"`
	ReportPath string `json:"reportPath,omitempty" yaml:"reportPath,omitempty" bson:"-"`
}

type TestStat struct {
	Description string `bson:"Description"`
	// Total Count of execution
	Count int `bson:"Count"`
	// succefully executed
	Executed int `bson:"Executed"`
	// failed due to error
	Fail int `bson:"Fail"`

	// latency
	TotalLatency time.Duration  `json:"-" yaml:"-" bson:"-"`
	AvgLatency   *time.Duration `json:",omitempty" yaml:",omitempty" bson:"AvgLatency"`
	MaxLatency   *time.Duration `json:",omitempty" yaml:",omitempty" bson:"MaxLatency"`
	MinLatency   *time.Duration `json:",omitempty" yaml:",omitempty" bson:"MinLatency"`

	L99thLatency *time.Duration `json:",omitempty" yaml:",omitempty" bson:"L99thLatency"`
	L95thLatency *time.Duration `json:",omitempty" yaml:",omitempty" bson:"L95thLatency"`
	L90thLatency *time.Duration `json:",omitempty" yaml:",omitempty" bson:"L90thLatency"`

	// This is for computing 90th, 95th, 99th percentile
	Raw []time.Duration `json:"-" yaml:"-" bson:"-"`
}

// any override for test
type TestOverride struct {
	Mock []*MockOverride `json:"mock,omitempty" yaml:"mock,omitempty"`
}

// mock response override
type MockOverride struct {
	EndpointName   string         `json:"endpointName" yaml:"endpointName"`
	MethodName     string         `json:"methodName" yaml:"methodName"`
	Value          *Value         `json:",inline" yaml:",inline"`
	ResponseConfig *TrafficConfig `json:"responseConfig,omitempty" yaml:"responseConfig,omitempty"`
}

// db related params
type DBParam struct {
	Database string   `json:"database" yaml:"database"`
	Table    string   `json:"table,omitempty" yaml:"table,omitempty"`
	Tables   []string `json:"tables,omitempty" yaml:"tables,omitempty"`
}

// mongo read params
type MongoReadParam struct {
	Aggregate string `json:"aggregate,omitempty" yaml:"aggregate,omitempty"`
	Filter    string `json:"filter,omitempty" yaml:"filter,omitempty"`
	Fields    string `json:"fields,omitempty" yaml:"fields,omitempty"`
	Sort      string `json:"sort,omitempty" yaml:"sort,omitempty"`
}

// mongo update params
type MongoUpdateParam struct {
	Filter string `json:"filter,omitempty" yaml:"filter,omitempty"`
	Update string `json:"update,omitempty" yaml:"update,omitempty"`
}

type WorkerBackend string

const (
	WorkerBackendKubernetes = "kubernetes"
	WorkerBackendDocker     = "docker"
	WorkerBackendStandAlone = "standalone"
)

type WorkerBootConfig struct {
	// Represents the container backend where the worker is running (e.g. kubernetes)
	Backend WorkerBackend `json:"backend,omitempty" yaml:"backend,omitempty"`
	// management port
	MgmtPort int `json:"managementPort,omitempty" yaml:"managementPort,omitempty"`
}

type MongoConfig struct {
	Enabled              bool   `json:"enabled" yaml:"enabled"`
	MongodbUrl           string `json:"mongodbUrl" yaml:"mongodbUrl"`
	Database             string `json:"database" yaml:"database"`
	TestResultCollection string `json:"testResultCollection" yaml:"testResultCollection"`
	MockCollection       string `json:"mockCollection" yaml:"mockCollection"`
}

type PrometheusConfig struct {
	Enabled        bool   `json:"enabled" yaml:"enabled"`
	PushGatewayUrl string `json:"pushGatewayUrl" yaml:"pushGatewayUrl"`
}

type ManagementPlaneConfig struct {
	MongoConfig      MongoConfig      `json:"mongoConfig,omitempty" yaml:"mongoConfig,omitempty"`
	PrometheusConfig PrometheusConfig `json:"prometheusConfig,omitempty" yaml:"prometheusConfig,omitempty"`
}

type DebugRuntimeType string

const (
	DebugRuntimeGo      = "go"
	DebugRuntimeNode    = "node"
	DebugRuntimeJava    = "jvm"
	DebugRuntimePython  = "python"
	DebugRuntimeUnknown = "unknown"
)

// struct that stores deployer debugging related info
type DebugInfo struct {
	// container to attach debugger
	ContainerName string `json:"containerName" yaml:"containerName"`
	// optional: any local paths to mount
	MountPaths []string `json:"mountPaths,omitempty" yaml:"mountPaths,omitempty"`
	// optional: port for debugger
	Port int `json:"debugPort,omitempty" yaml:"debugPort,omitempty"`
	// optional: runtime
	RuntimeType DebugRuntimeType `json:"runtimeType,omitempty" yaml:"runtimeType,omitempty"`
	// optional: override command
	Cmd string `json:"command,omitempty" yaml:"command,omitempty"`
	// optional: override args
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`
}

type Mock struct {
	// Identifier of this mock
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Mock responses contains info of response to be included in the mock with traffic configuration
	Responses []*ResponseConfig `json:"responses,omitempty" yaml:"responses,omitempty"`
	// Configuration for traffic
	*TrafficConfig `json:",inline" yaml:",inline"`
	// If endpoint and method are added to the proxies list, it means we are not mocking that method endpoint, but forwarding request to that endpoint method
	Proxies []*Proxy `json:"proxies,omitempty" yaml:"proxies,omitempty"`
}

type Proxy struct {
	// endpointName is the name of the endpoint that this proxy is associated with
	EndpointName string `json:"endpointName,omitempty" yaml:"endpointName,omitempty"`
	// methodName is the name of the method that this proxy is associated with
	MethodName string `json:"methodName,omitempty" yaml:"methodName,omitempty"`
}

type HelmOptions struct {
	// Chart is a chart reference for a local or remote chart.
	Chart string
	// Repo is a URL to a custom chart repository.
	Repo string
	// Version is the version of the chart to fetch.
	Version string
}
