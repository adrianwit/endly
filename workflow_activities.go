package endly

import "strings"

//Activities represents a workflow activities
type Activities []*WorkflowServiceActivity

//Push adds a workflow to the workflow stack.
func (a *Activities) Push(workflow *WorkflowServiceActivity) {
	*a = append(*a, workflow)
}

//Pop removes the first workflow from the workflow stack.
func (a *Activities) Pop() *WorkflowServiceActivity {
	if len(*a) == 0 {
		return nil
	}
	var result = (*a)[len(*a)-1]

	if len(*a) > 0 {
		(*a) = (*a)[:len(*a)-1]
	}
	return result
}

//Last returns the last workflow from the workflow stack.
func (a *Activities) Last() *WorkflowServiceActivity {
	if a == nil {
		return nil
	}
	var workflowCount = len(*a)
	if workflowCount == 0 {
		return nil
	}
	return (*a)[workflowCount-1]
}

//GetPath returns hierarchical path to the latest Activity
func (a *Activities) GetPath(runner *CliRunner, fullPath bool) (string, int) {
	var pathLength = 0
	var activityPath = make([]string, 0)

	for i, activity := range *a {
		var tag = activity.FormatTag()
		pathLength += len(tag)

		serviceAction := ""
		if i+1 < len(*a) || fullPath {
			serviceAction = runner.ColorText(activity.Service+"."+activity.Action, runner.ServiceActionColor)
			pathLength += len(activity.Service) + 1 + len(activity.Action)
		}

		tag = runner.ColorText(tag, runner.TagColor)
		if runner.InverseTag {
			tag = runner.ColorText(tag, "inverse")
		}
		activityPath = append(activityPath, runner.ColorText(activity.Workflow, runner.PathColor)+tag+serviceAction)
		pathLength += len(activity.Workflow)
	}

	var path = strings.Join(activityPath, runner.ColorText("|", "gray"))
	if len(*a) > 0 {
		pathLength += (len(*a) - 1)
	}
	return path, pathLength + 1
}

//AsyncServiceActionEvent represent async action
type AsyncServiceActionEvent struct {
	Workflow    string
	Task        string
	Service     string
	Action      string
	TagID       string
	Description string
}

//NewAsyncServiceActionEvent return new AsyncServiceActionEvent
func NewAsyncServiceActionEvent(workflow string, task string, service string, action string, tagID string, description string) *AsyncServiceActionEvent {
	return &AsyncServiceActionEvent{
		Workflow:    workflow,
		Task:        task,
		Service:     service,
		Action:      action,
		TagID:       tagID,
		Description: description,
	}
}
