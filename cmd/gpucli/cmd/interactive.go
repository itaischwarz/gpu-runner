package cmd

import (
	"fmt"
	"gpu-runner/internal/ui"
	
)

func runInteractiveMode() error {
	for {
		action, err := ui.PromptForAction()
		if err != nil {
			return err
		}
		switch action {
		case ui.ActionList:
			if err := handleListAction(); err != nil {
				return fmt.Errorf("list jobs: %w", err)
			}
		case ui.ActionStart:
			if err := 	handleStartAction(); err != nil{
				return fmt.Errorf("start job: %w", err)
		}
		case ui.ActionStatus:
            if err := handleStatusAction(); err != nil {
                return fmt.Errorf("status job: %w", err)
                }
		case ui.ActionCancel:
        if err := handleCancelAction(); err != nil {
          return fmt.Errorf("cancel job: %w", err)
        }
		case ui.ActionStop:
				if err := handleStopAction(); err != nil {
					return fmt.Errorf("stop gpu: %w", err)
				}
    }
		cont, err := ui.PromptToContinue()
    if err != nil || !cont {
          break
    }
		
	
	}
	return nil
}

func handleListAction() error {
    confirm, err := ui.PromptForConfirmation("Would you like to filter by status?")
    if err != nil {
        return err
    }
    var status string
    if confirm {
        jobStatus, err := ui.PromptForStatus("Select status to filter:")
        if err != nil {
            return err
        }
        status = string(jobStatus)
    }
    return listJobs(status)
}
	

func handleStartAction() error {
    command, err := ui.PromptCommand()
    if err != nil {
        return err 
    }

    storage := ""
    confirm, err := ui.PromptForConfirmation("Would you like to specify storage?")
    if err != nil {
        return err
    }
    if confirm {
        storage, err = ui.PromptStorage()
        if err != nil {
            return err
        }
    }

    maxRetries := ""
    confirm, err = ui.PromptForConfirmation("Would you like to set max retries?")
    if err != nil {
        return err
    }
    if confirm {
        maxRetries, err = ui.PromptMaxRetries()
        if err != nil {
            return err
        }
    }


    return submitJob(command, storage, maxRetries)
}

func handleStopAction() error {
	reason, err := ui.PromptForReason()
	if err != nil {
		return err
	}
	return shutdownServer(reason)
}

func handleCancelAction() error {
    activeJobIDs, err := ui.FetchActiveJobs("http://0.0.0.0:8080")
	jobID, err := ui.PromptForJobId(activeJobIDs)
	if err != nil {
		return err
	}
	reason, err := ui.PromptForReason()
	if err != nil {
		return err
	}

	return cancelJob(jobID, reason)
}

func handleStatusAction() error {
	jobID, err := ui.PromptForJobId(make([]string, 0))
	if err != nil {
		return err
	}
	return checkJobStatus(jobID)
}

