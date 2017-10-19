package endly

import (
	"errors"
	"fmt"
	"github.com/viant/endly/common"
	"github.com/viant/toolbox"
	"strings"
	"time"
)

const (
	WorkflowServiceId                = "workflow"
	WorkflowEvalRunCriteriaEventType = "EvalRunCriteria"
)

type WorkflowRunRequest struct {
	WorkflowURL       string
	Name              string
	Params            map[string]interface{}
	Tasks             string
	PublishParameters bool //publishes parameters name into context state
	Async             bool //flag to run it asynchronously
}

type WorkflowRunResponse struct {
	Data      map[string]interface{}
	SessionId string
}

type WorkflowServiceActivity struct {
	Service         string
	Action          string
	Error           string
	Skipped         string
	ServiceRequest  interface{}
	ServiceResponse interface{}
}

type WorkflowRegisterRequest struct {
	Workflow *Workflow
}

type WorkflowLoadRequest struct {
	Source *Resource
}

type WorkflowService struct {
	*AbstractService
	dao      *WorkflowDao
	registry map[string]*Workflow
}

func (s *WorkflowService) Register(workflow *Workflow) error {
	err := workflow.Validate()
	if err != nil {
		return err
	}
	s.registry[workflow.Name] = workflow
	return nil
}

func (s *WorkflowService) HasWorkflow(name string) bool {
	_, found := s.registry[name]
	return found
}

func (s *WorkflowService) Workflow(name string) (*Workflow, error) {
	if result, found := s.registry[name]; found {
		return result, nil
	}
	return nil, fmt.Errorf("Failed to lookup workflow: %v", name)
}

func (s *WorkflowService) evaluateRunCriteria(context *Context, criteria string) (bool, error) {
	if criteria == "" {
		return true, nil
	}

	colonPosition := strings.Index(criteria, ":")
	if colonPosition == -1 {
		return true, nil
	}
	fragments := strings.Split(criteria, ":")

	actualOperand := context.Expand(strings.TrimSpace(fragments[0]))
	expectedOperand := context.Expand(strings.TrimSpace(fragments[1]))
	validator := &Validator{}
	var result, err = validator.Check(expectedOperand, actualOperand)
	s.AddEvent(context, WorkflowEvalRunCriteriaEventType, Pairs("actual", actualOperand, "expected", expectedOperand, "eligible", result), Finest)
	return result, err
}

func isTaskAllowed(candidate *WorkflowTask, request *WorkflowRunRequest) (bool, map[int]bool) {

	if request.Tasks == "" {
		return true, nil
	}
	var actions map[int]bool
	var encodedTask []string
	tasks := strings.Split(request.Tasks, ",")
	for _, task := range tasks {
		encodedTask = nil
		var taskName = task
		if !strings.Contains(task, "=") {
			encodedTask = strings.Split(task, "=")
			taskName = encodedTask[0]
		}
		if taskName == candidate.Name {
			if len(encodedTask) == 2 {
				actions = make(map[int]bool)
				for _, allowedIndex := range strings.Split(encodedTask[1], ":") {
					actions[toolbox.AsInt(allowedIndex)] = true
				}
			}
			return true, actions
		}
	}
	return false, nil
}

func (s *WorkflowService) loadWorkflowIfNeeded(context *Context, name string, URL string) (err error) {
	if !s.HasWorkflow(name) {
		var workflowResource *Resource
		if URL != "" {
			workflowResource, err = NewResource(URL)
			if err != nil {
				return err
			}
		} else {
			workflowResource, err = NewEndlyRepoResource(context, fmt.Sprintf("workflow/%v.csv", name))
			if err != nil {
				return err
			}
		}
		if err == nil {
			err = s.loadWorkflow(context, &WorkflowLoadRequest{Source: workflowResource})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *WorkflowService) runAction(context *Context, action *ServiceAction) error {
	var state = context.state
	serviceActivity := &WorkflowServiceActivity{
		Action:  action.Action,
		Service: action.Service,
	}
	state.Put("activity", serviceActivity)

	canRun, err := s.evaluateRunCriteria(context, action.RunCriteria)
	if err != nil {
		return err
	}
	if !canRun {
		serviceActivity.Skipped = fmt.Sprintf("Does not match run criteria: %v", context.Expand(action.RunCriteria))
		return nil
	}

	err = action.Init.Apply(state, state)
	if err != nil {
		return err
	}
	service, err := context.Service(action.Service)

	if err != nil {
		return err
	}

	expandedRequest := ExpandValue(action.Request, state)
	if expandedRequest == nil || !toolbox.IsMap(expandedRequest) {
		return fmt.Errorf("Failed to exaluate request: %v, expected map but had: %T", expandedRequest, expandedRequest)
	}
	requestMap := toolbox.AsMap(expandedRequest)
	serviceRequest, err := service.NewRequest(action.Action)
	if err != nil {
		return err
	}

	serviceActivity.ServiceRequest = serviceRequest
	err = converter.AssignConverted(serviceRequest, requestMap)
	if err != nil {
		return fmt.Errorf("Failed to create request %v on %v.%v, %v", requestMap, action.Service, action.Action, err)
	}
	var responseMap = make(map[string]interface{})
	startEvent := s.Begin(context, action, Pairs("service",  action.Service, "action", action.Action, "group", action.Group, "subPath", action.Subpath, "description", action.Description, "request", requestMap), Info)

	defer s.End(context)(startEvent, Pairs("response", responseMap))
	serviceResponse := service.Run(context, serviceRequest)
	serviceActivity.ServiceResponse = serviceResponse

	if serviceResponse.Error != "" {
		var err = reportError(errors.New(serviceResponse.Error))
			return err
	}


	if serviceResponse.Response != nil {
		converter.AssignConverted(responseMap, serviceResponse.Response)
	}
	err = action.Post.Apply(common.Map(responseMap), state) //result to task  state
	if err != nil {
		return err
	}
	if action.SleepInMs > 0 {
		s.AddEvent(context, SleepEventType, Pairs("sleepTime", action.SleepInMs), Info)
		time.Sleep(time.Millisecond * time.Duration(action.SleepInMs))
	}
	return nil
}



func (s *WorkflowService) runTask(context *Context, task *WorkflowTask, request *WorkflowRunRequest) error {
	var state = context.state
	state.Put("task", task)
	var taskAllowed, allowedServiceActions = isTaskAllowed(task, request)
	if !taskAllowed {
		return nil
	}
	var hasAllowedActions = len(allowedServiceActions) > 0
	err := task.Init.Apply(state, state)
	if err != nil {
		return err
	}

	canRun, err := s.evaluateRunCriteria(context, task.RunCriteria)
	if err != nil {
		return err
	}
	if !canRun {
		return nil
	}
	startEvent := s.Begin(context, task, Pairs("name", task.Name))
	defer s.End(context)(startEvent, Pairs())

	var group = ""
	for i, action := range task.Actions {
		if hasAllowedActions && !allowedServiceActions[i] {
			continue
		}
		if action.Group != "" {
			if group != action.Group {
				s.AddEvent(context, "ActionGroup", Pairs( "name", action.Group, "description", action.Description, "subPath", action.Subpath), Info)
			}
			group = action.Group
		}
		err = s.runAction(context, action)
		if err != nil {
			return err
		}
	}
	return task.Post.Apply(state, state)
}

func (s *WorkflowService) runWorkflow(upstreamContext *Context, request *WorkflowRunRequest) (*WorkflowRunResponse, error) {
	var err = s.loadWorkflowIfNeeded(upstreamContext, request.Name, request.WorkflowURL)
	if err != nil {
		return nil, err
	}
	workflow, err := s.Workflow(request.Name)
	if err != nil {
		return nil, err
	}
	upstreamContext.PushWorkflow(workflow)
	defer upstreamContext.ShiftWorkflow()

	var response = &WorkflowRunResponse{
		SessionId: upstreamContext.SessionId,
		Data:      make(map[string]interface{}),
	}

	parentWorkflow := upstreamContext.Workflow()
	if parentWorkflow != nil {
		upstreamContext.Put(workflowKey, parentWorkflow)
	} else {
		upstreamContext.Put(workflowKey, workflow)
	}

	context := upstreamContext.Clone()
	var state = context.State()

	var workflowData = common.Map(workflow.Data)
	state.Put("workflow", workflowData)

	params := buildParamsMap(request, context)

	if request.PublishParameters {
		for key, value := range params {
			state.Put(key, ExpandValue(value, state))
		}
	}
	state.Put("params", params)
	err = workflow.Init.Apply(state, state)
	if err != nil {
		return nil, err
	}

	for _, task := range workflow.Tasks {
		err = s.runTask(context, task, request)
		if err != nil {
			return nil, err
		}
	}
	state.Delete("activity")
	workflow.Post.Apply(state, response.Data) //context -> workflow output
	return response, nil
}


func buildParamsMap(request *WorkflowRunRequest, context *Context) common.Map {
	var params = common.NewMap()
	if len(request.Params) > 0 {
		for k, v := range request.Params {
			if toolbox.IsString(v) {
				params[k] = context.Expand(toolbox.AsString(v))
			} else {
				params[k] = v
			}
		}
	}
	return params
}

func (s *WorkflowService) loadWorkflow(context *Context, request *WorkflowLoadRequest) error {
	workflow, err := s.dao.Load(context, request.Source)
	if err != nil {
		return fmt.Errorf("Failed to load workflow: %v, %v", request.Source, err)
	} else {
		err = s.Register(workflow)
		if err != nil {
			return fmt.Errorf("Failed to register workflow: %v, %v", request.Source, err)
		}
	}
	return nil
}

func (s *WorkflowService) removeSession(context *Context) {
	time.Sleep(1 * time.Second)
	s.Mutex().Lock()
	defer s.Mutex().Unlock()
	s.state.Delete(context.SessionId)
}

func (s *WorkflowService) startSession(context *Context) bool {
	s.Mutex().RLock()
	if s.state.Has(context.SessionId) {
		s.Mutex().RUnlock()
		return false
	}
	s.Mutex().RUnlock()
	s.state.Put(context.SessionId, context)
	s.Mutex().Lock()
	defer s.Mutex().Unlock()
	return true
}

func (s *WorkflowService) isAsyncRequest(request interface{}) bool {
	if runRequest, ok := request.(*WorkflowRunRequest);ok {
		return runRequest.Async
	}
	return false
}

func (s *WorkflowService) reportErrorIfNeeded(context *Context, response *ServiceResponse) {
	if response.Error != "" {
		s.AddEvent(context, ErrorEventType, Pairs("error", response.Error), Info)
	}
}

func (s *WorkflowService) Run(context *Context, request interface{}) *ServiceResponse {
	startedSession := s.startSession(context)
	startEvent := s.Begin(context, request, Pairs("request", request))
	var response = &ServiceResponse{Status: "ok"}
	if ! s.isAsyncRequest(request) {
		defer s.reportErrorIfNeeded(context, response)
		defer s.End(context)(startEvent, Pairs("response", response))
	}
	var err error
	switch actualRequest := request.(type) {
	case *WorkflowRunRequest:
		if actualRequest.Async {
			go func() {
				if startedSession {
					defer s.reportErrorIfNeeded(context, response)
					defer s.removeSession(context)
				}
				_, err:= s.runWorkflow(context, actualRequest)
				fmt.Printf("Completed async workflow\n")
				if err != nil {
					s.AddEvent(context, ErrorEventType, Pairs("error", err), Info)
				}
				s.End(context)(startEvent, Pairs("response", response))
			}()

			response.Response = &WorkflowRunResponse{
				SessionId: context.SessionId,
			}
			return response
		}
		response.Response, err = s.runWorkflow(context, actualRequest)
		if err != nil {
			response.Error = fmt.Sprintf("Failed to run workflow: %v, %v", actualRequest.Name, err)
		}
	case *WorkflowRegisterRequest:
		err := s.Register(actualRequest.Workflow)
		if err != nil {
			response.Error = fmt.Sprintf("Failed to register workflow: %v, %v", actualRequest.Workflow.Name, err)
		}
	case *WorkflowLoadRequest:
		err := s.loadWorkflow(context, actualRequest)
		if err != nil {
			response.Error = fmt.Sprintf("%v", err)
		}
	default:
		response.Error = fmt.Sprintf("Unsupported request type: %T", request)
	}
	if response.Error != "" {
		response.Status = "err"
	}
	return response
}

func (s *WorkflowService) NewRequest(action string) (interface{}, error) {
	switch action {
	case "run":
		return &WorkflowRunRequest{}, nil
	case "register":
		return &WorkflowRegisterRequest{}, nil
	case "load":
		return &WorkflowLoadRequest{}, nil
	}
	return s.AbstractService.NewRequest(action)
}

func NewWorkflowService() Service {
	var result = &WorkflowService{
		AbstractService: NewAbstractService(WorkflowServiceId),
		dao:             NewWorkflowDao(),
		registry:        make(map[string]*Workflow),
	}
	result.AbstractService.Service = result
	return result
}