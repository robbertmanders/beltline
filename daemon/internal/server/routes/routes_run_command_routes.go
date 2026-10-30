// The run commands and the
// task start, one registration function per endpoint (see the registry for each
// method and path). The run reads are in routes_run_routes.go.
package routes

import (
	"net/http"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// runCommandRoute registers a POST run command on pattern. The run named by the
// id segment must exist (a 404 before the body is read), then the body is read
// (required or optional), then command runs; the response is the run after the
// command, with the actions the daemon would accept then.
func runCommandRoute(mux *http.ServeMux, pattern string, c *coordinator.Coordinator, required bool,
	command func(r *http.Request, id string, body []byte) (core.Run, error)) {
	mux.Handle(pattern, ErrorRoute(func(w http.ResponseWriter, r *http.Request) error {
		id, body, err := runPrepared(c, r, required)
		if err != nil {
			return err
		}
		run, err := command(r, id, body)
		if err != nil {
			return err
		}
		JSONResponse(w, api.RunResponse{Run: runSnapshot(r.Context(), run, c)}, http.StatusOK)
		return nil
	}))
}

// AddRunDecide POST api/v1/runs/{id}/decide.
func AddRunDecide(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/decide", c, true, func(_ *http.Request, id string, body []byte) (core.Run, error) {
		decoded, err := runDecode[api.RunDecideRequest](body)
		if err != nil {
			return core.Run{}, err
		}
		return c.DecideCommand(id, decoded.Outcome, decoded.Note)
	})
}

// AddRunResume POST api/v1/runs/{id}/resume.
// Earlier is true unless the body says otherwise.
func AddRunResume(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/resume", c, true, func(_ *http.Request, id string, body []byte) (core.Run, error) {
		decoded, err := runDecode[api.RunResumeRequest](body)
		if err != nil {
			return core.Run{}, err
		}
		earlier := core.ValueOr(decoded.Earlier, true)
		return c.ResumeCommand(id, decoded.Stage, decoded.Note, earlier, decoded.Leg)
	})
}

// AddRunAnswer POST api/v1/runs/{id}/answer.
func AddRunAnswer(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/answer", c, true, func(_ *http.Request, id string, body []byte) (core.Run, error) {
		decoded, err := runDecode[api.RunAnswerRequest](body)
		if err != nil {
			return core.Run{}, err
		}
		return c.AnswerCommand(id, decoded.Answer)
	})
}

// AddRunAbort POST api/v1/runs/{id}/abort.
// The body is optional and names the leg.
func AddRunAbort(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/abort", c, false, func(_ *http.Request, id string, body []byte) (core.Run, error) {
		decoded, err := runDecodeOptional[api.RunLegRequest](body)
		if err != nil {
			return core.Run{}, err
		}
		return c.AbortCommand(id, legOf(decoded))
	})
}

// AddRunDrop POST api/v1/runs/{id}/drop.
// The body is optional and names the leg.
func AddRunDrop(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/drop", c, false, func(_ *http.Request, id string, body []byte) (core.Run, error) {
		decoded, err := runDecodeOptional[api.RunLegRequest](body)
		if err != nil {
			return core.Run{}, err
		}
		return c.DropCommand(id, legOf(decoded))
	})
}

// AddRunPark POST api/v1/runs/{id}/park.
func AddRunPark(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/park", c, false, func(_ *http.Request, id string, _ []byte) (core.Run, error) {
		return c.ParkCommand(id)
	})
}

// AddRunUnpark POST api/v1/runs/{id}/unpark.
func AddRunUnpark(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/unpark", c, false, func(_ *http.Request, id string, _ []byte) (core.Run, error) {
		return c.UnparkCommand(id)
	})
}

// AddRunPause POST api/v1/runs/{id}/pause.
func AddRunPause(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/pause", c, false, func(r *http.Request, id string, _ []byte) (core.Run, error) {
		return c.PauseRunCommand(r.Context(), id)
	})
}

// AddRunResumeRun POST api/v1/runs/{id}/resume-run.
func AddRunResumeRun(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/resume-run", c, false, func(r *http.Request, id string, _ []byte) (core.Run, error) {
		return c.ResumeRunCommand(r.Context(), id)
	})
}

// AddRunRetryContinuation is the add run retry continuation:
// POST api/v1/runs/{id}/retry-continuation. The body is optional and names the leg.
func AddRunRetryContinuation(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/retry-continuation", c, false, func(r *http.Request, id string, body []byte) (core.Run, error) {
		decoded, err := runDecodeOptional[api.RunLegRequest](body)
		if err != nil {
			return core.Run{}, err
		}
		return c.RetryContinuationCommand(r.Context(), id, legOf(decoded))
	})
}

// AddRunStartNow POST api/v1/runs/{id}/start-now.
func AddRunStartNow(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/start-now", c, false, func(r *http.Request, id string, _ []byte) (core.Run, error) {
		return c.StartNowCommand(r.Context(), id)
	})
}

// AddRunContinueSession is the add run continue session:
// POST api/v1/runs/{id}/continue-session. The response names the writable tmux
// session the command attaches, which the daemon resolves so the browser never
// parses the command.
func AddRunContinueSession(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/runs/{id}/continue-session", ErrorRoute(func(w http.ResponseWriter, r *http.Request) error {
		id, body, err := runPrepared(c, r, true)
		if err != nil {
			return err
		}
		decoded, err := runDecode[api.RunAttemptRequest](body)
		if err != nil {
			return err
		}
		run, command, err := c.ContinueSessionCommand(r.Context(), id, decoded.Attempt)
		if err != nil {
			return err
		}
		target := coordinator.SessionSidebarProjectionAttachTarget(command)
		if target == nil {
			return core.NewFailure(core.RunAvailabilitySessionGone)
		}
		JSONResponse(w, api.ContinueSessionResponse{Run: runSnapshot(r.Context(), run, c), Command: command, Target: *target}, http.StatusOK)
		return nil
	}))
}

// AddRunAdjustStage POST api/v1/runs/{id}/adjust-stage.
func AddRunAdjustStage(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/adjust-stage", c, true, func(_ *http.Request, id string, body []byte) (core.Run, error) {
		decoded, err := runDecode[api.RunAdjustStageRequest](body)
		if err != nil {
			return core.Run{}, err
		}
		thinking, err := runDecodeThinking(decoded.Thinking)
		if err != nil {
			return core.Run{}, err
		}
		runner, err := runDecodeBackend(decoded.Runner)
		if err != nil {
			return core.Run{}, err
		}
		return c.AdjustStageCommand(id, decoded.Stage, decoded.Model, thinking, decoded.Brief, runner)
	})
}

// AddRunAdjustSwitch is the add run adjust switch:
// POST api/v1/runs/{id}/adjust/switch. It answers the runner, model and effort
// the switch resolves to; it writes nothing.
func AddRunAdjustSwitch(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/runs/{id}/adjust/switch", ErrorRoute(func(w http.ResponseWriter, r *http.Request) error {
		id, body, err := runPrepared(c, r, true)
		if err != nil {
			return err
		}
		decoded, err := runDecode[api.RunAdjustSwitchRequest](body)
		if err != nil {
			return err
		}
		runner, err := runDecodeBackend(&decoded.Runner)
		if err != nil {
			return err
		}
		if runner == nil {
			return core.NewValidationFailure("runner must name a supported backend.", nil)
		}
		thinking, err := runDecodeThinking(decoded.Thinking)
		if err != nil {
			return err
		}
		model := core.ValueOf(decoded.Model)
		draft := core.StageAdjustDraft{Runner: *runner, Model: model, Thinking: thinking, Brief: ""}
		next, err := c.AdjustSwitchCommand(id, decoded.Stage, draft)
		if err != nil {
			return err
		}
		var nextThinking *string
		if next.Thinking != nil {
			level := string(*next.Thinking)
			nextThinking = &level
		}
		JSONResponse(w, api.RunAdjustSwitchResponse{Runner: string(next.Runner), Model: next.Model, Thinking: nextThinking}, http.StatusOK)
		return nil
	}))
}

// AddRunAfk POST api/v1/runs/{id}/afk.
// The mode is always, never or global.
func AddRunAfk(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/afk", c, true, func(r *http.Request, id string, body []byte) (core.Run, error) {
		decoded, err := runDecode[api.RunAfkRequest](body)
		if err != nil {
			return core.Run{}, err
		}
		mode, ok := runAfkMode(decoded.Mode)
		if !ok {
			return core.Run{}, core.NewValidationFailure("mode must be always, never or global.", nil)
		}
		return c.AfkCommand(r.Context(), id, mode)
	})
}

// AddRunErase POST api/v1/runs/{id}/erase.
// An erased id answers again (the erase is repeatable); an id never seen is a 404.
func AddRunErase(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/runs/{id}/erase", JSONRoute(c, func(r *http.Request) (any, error) {
		id, err := ReadRouteSupportID(r)
		if err != nil {
			return nil, err
		}
		if _, err := ReadRouteSupportCommandBody(r, false); err != nil {
			return nil, err
		}
		live := c.ReadRun(id) != nil
		erasedBefore := c.ErasedID(id)
		if !live && !erasedBefore {
			return nil, api.MissingResource{}
		}
		if err := c.EraseCommand(r.Context(), id); err != nil {
			return nil, err
		}
		return api.EraseResponse{ID: id, Erased: true}, nil
	}))
}

// AddRunOverruleAfk POST
// api/v1/runs/{id}/overrule-afk. The run must exist; the decision names the
// applied AFK decision the run restarts from.
func AddRunOverruleAfk(mux *http.ServeMux, c *coordinator.Coordinator) {
	runCommandRoute(mux, "POST /api/v1/runs/{id}/overrule-afk", c, true, func(r *http.Request, _ string, body []byte) (core.Run, error) {
		decoded, err := runDecode[api.RunOverruleAfkRequest](body)
		if err != nil {
			return core.Run{}, err
		}
		return c.OverruleAfkCommand(r.Context(), decoded.Decision, decoded.Stage, decoded.Comments, decoded.AcceptedWarning)
	})
}

// AddTasks POST api/v1/tasks, which starts a task on a manual belt.
func AddTasks(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/tasks", JSONRoute(c, func(r *http.Request) (any, error) {
		body, err := ReadRouteSupportCommandBody(r, true)
		if err != nil {
			return nil, err
		}
		decoded, err := runDecode[api.RunTaskRequest](body)
		if err != nil {
			return nil, err
		}
		run, err := c.StartTaskCommand(decoded.Belt, decoded.Repo, decoded.Title, decoded.Body, decoded.Entry)
		if err != nil {
			return nil, err
		}
		return api.RunResponse{Run: runSnapshot(r.Context(), run, c)}, nil
	}))
}
