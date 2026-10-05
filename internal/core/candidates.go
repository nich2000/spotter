package core

import "time"

func (s *State) candidateCommand(c Command, now time.Time) (Object, error) {
	p := c.Payload
	if err := checkKeys(p, "candidateId", "candidateVersion", "sourceSnapshotId", "card"); err != nil {
		return nil, err
	}
	id := str(p, "candidateId")
	candidate, err := requiredEntity(s.Candidates, id)
	if err != nil {
		return nil, err
	}
	if number(candidate, "candidateVersion") != number(p, "candidateVersion") {
		return nil, Fail("CANDIDATE_CHANGED", 409, "Предложение изменилось")
	}
	if snapshot, supplied := p["sourceSnapshotId"]; supplied && snapshot != candidate["sourceSnapshotId"] {
		return nil, Fail("CANDIDATE_CHANGED", 409, "Источник предложения изменился")
	}
	if candidate["state"] == "accepted" {
		return nil, Fail("CANDIDATE_ALREADY_ACCEPTED", 409, "Предложение уже принято")
	}
	before := Copy(candidate)
	switch c.Command {
	case "source.accept":
		payload := Object{"title": candidate["proposedTitle"], "projectId": nil}
		if card := obj(p, "card"); card != nil {
			for k, v := range card {
				payload[k] = v
			}
			if patch := obj(payload, "patch"); patch != nil {
				if v, ok := patch["title"]; ok {
					payload["title"] = v
				}
				if v, ok := patch["projectId"]; ok {
					payload["projectId"] = v
				}
			}
		}
		result, err := s.taskCommand(Command{OperationID: c.OperationID, Command: "task.create", Payload: payload}, now)
		if err != nil {
			return nil, err
		}
		candidate["state"] = "accepted"
		candidate["acceptedOccurrenceId"] = result["taskId"]
		s.Tasks[result["taskId"].(string)]["sourceCandidateId"] = id
		result["candidateId"] = id
		s.event(c.Command, id, c.OperationID, before, candidate, now)
		return result, nil
	case "source.dismiss":
		candidate["state"] = "dismissed"
	case "source.restore":
		candidate["state"] = "pending"
	default:
		return nil, Fail("UNKNOWN_COMMAND", 400, "Неизвестная команда источника")
	}
	s.event(c.Command, id, c.OperationID, before, candidate, now)
	return Object{"candidateId": id, "state": candidate["state"]}, nil
}
