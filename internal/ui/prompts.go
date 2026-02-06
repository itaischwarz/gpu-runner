package ui

import (
	"github.com/AlecAivazis/survey/v2"
	"gpu-runner/internal/jobs"
)

type Action string 

const (
    ActionStatus Action = "status"
    ActionStart  Action = "start"
    ActionStop   Action = "stop"
    ActionList   Action = "list"
		ActionCancel Action = "cancel"
)


func PromptForAction() (Action, error) {
	var action string 
	prompt := &survey.Select{
		Message: "What would you like to do?",
		Options: []string{
		"Check Job Status",
		"Start Job",
		"Stop GPU",
		"Cancel Job",
		"List Jobs",
		},
	}

	err := survey.AskOne(prompt, &action )
	if err != nil {
		return "", err 
	}

	 actionMap := map[string] Action{
		"Check Job Status": ActionStatus,
		"Start Job" : ActionStart ,
		"Stop GPU" : ActionStart ,
		"Cancel Job": ActionCancel,
		"List Jobs":     ActionList, 
		    
	}
	
	return actionMap[action], nil 
}

func PromptForJobId(JobIDs []string) (string, error){
	var jobID string 
	var prompt survey.Prompt
	if len(JobIDs) == 0 {
		prompt = &survey.Input{
			Message: "Select Job ID",
		}
	} else {
		prompt = &survey.Select{
			Message: "Select Job ID",
			Options: JobIDs,
		}
	}
	err := survey.AskOne(prompt, &jobID, survey.WithValidator(survey.Required))
	return jobID, err 
}


func PromptForConfirmation(message string) (bool, error) {
    var confirm bool
    prompt := &survey.Confirm{
        Message: message,
				Default: true,
    }
    
    err := survey.AskOne(prompt, &confirm)
    return confirm, err
}

func PromptForStatus(message string) (jobs.JobStatus, error){
		var statusStr string 
    prompt := &survey.Select{
        Message: message, 
        Options: []string{
            string(jobs.StatusCancelled),
            string(jobs.StatusFailed),
            string(jobs.StatusRunning),
            string(jobs.StatusSuccess),
            string(jobs.StatusPending),
        },
    }

		err := survey.AskOne(prompt, &statusStr)
		if err != nil{
			return "", err
		}
		return jobs.JobStatus(statusStr),  nil
}

// PromptToContinue asks if user wants to continue
func PromptToContinue() (bool, error) {
    return PromptForConfirmation("Would you like to perform another action?")
}

func PromptCommand() (string, error) {
	var command string 
	prompt := &survey.Input{
		Message: "Enter command to run on GPU:",
	}
	err := survey.AskOne(prompt, &command, survey.WithValidator(survey.Required))
	return command, err 
}

func PromptStorage() (string, error) {
	var storage string 
	prompt := &survey.Input{
		Message: "Enter storage volume (10, 25, 50 MB):",
	}
	err := survey.AskOne(prompt, &storage)
	return storage, err 
}

func PromptMaxRetries() (string, error) {
	var maxRetries string 
	prompt := &survey.Input{
		Message: "Enter max retries (default 0):",
	}
	err := survey.AskOne(prompt, &maxRetries)
	return maxRetries, err 
}

func PromptForReason() (string, error) {
	var reason string 
	prompt := &survey.Input{
		Message: "Enter reason:",
	}
	err := survey.AskOne(prompt, &reason, survey.WithValidator(survey.Required))
	return reason, err 
}


func PromptForPriority() (jobs.JobPriority, error) {
	var priorityStr string 
	prompt := &survey.Select{
		Message: "Select job priority:",
		Options: []string{
			string(jobs.ImmediatePriority),
			string(jobs.HighPriority),
			string(jobs.MediumPriority),
			string(jobs.LowPriority),
		},
	}
	err := survey.AskOne(prompt, &priorityStr)
	if err != nil {
		return "", err
	}
	return jobs.JobPriority(priorityStr), nil	
}
  