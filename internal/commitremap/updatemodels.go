package commitremap

func updatePullRequests(commitMap map[string]string, pullRequests *[]interface{}) error {
	for _, pr := range *pullRequests {
		if prMap, ok := pr.(map[string]interface{}); ok {
			// head_sha, base_sha, merge_commit_sha
			if headMap, ok := prMap["head"].(map[string]interface{}); ok {
				if headSha, ok := headMap["sha"].(string); ok {
					if newSha, ok := commitMap[headSha]; ok {
						headMap["sha"] = newSha
					}
				}
			}
			if baseMap, ok := prMap["base"].(map[string]interface{}); ok {
				if baseSha, ok := baseMap["sha"].(string); ok {
					if newSha, ok := commitMap[baseSha]; ok {
						baseMap["sha"] = newSha
					}
				}
			}
			if mergeCommitSha, ok := prMap["merge_commit_sha"].(string); ok {
				if newSha, ok := commitMap[mergeCommitSha]; ok {
					prMap["merge_commit_sha"] = newSha
				}
			}
		}
	}
	return nil
}

func updatePullRequestReviews(commitMap map[string]string, pullRequestReview *[]interface{}) error {
	for _, prr := range *pullRequestReview {
		if prrMap, ok := prr.(map[string]interface{}); ok {
			// head_sha
			if headSha, ok := prrMap["head_sha"].(string); ok {
				if newSha, ok := commitMap[headSha]; ok {
					prrMap["head_sha"] = newSha
				}
			}
		}
	}
	return nil
}

func updatePullRequestReviewComments(commitMap map[string]string, pullRequestReviewComments *[]interface{}) error {
	for _, prrc := range *pullRequestReviewComments {
		if prrcMap, ok := prrc.(map[string]interface{}); ok {
			// commit_id, original_commit_id
			if commitId, ok := prrcMap["commit_id"].(string); ok {
				if newSha, ok := commitMap[commitId]; ok {
					prrcMap["commit_id"] = newSha
				}
			}
			if originalCommitId, ok := prrcMap["original_commit_id"].(string); ok {
				if newSha, ok := commitMap[originalCommitId]; ok {
					prrcMap["original_commit_id"] = newSha
				}
			}
		}
	}
	return nil
}

func updatePullRequestReviewThreads(commitMap map[string]string, pullRequestReviewThreads *[]interface{}) error {
	for _, prrt := range *pullRequestReviewThreads {
		if prrtMap, ok := prrt.(map[string]interface{}); ok {
			// commit_id, original_commit_id
			if commitId, ok := prrtMap["commit_id"].(string); ok {
				if newSha, ok := commitMap[commitId]; ok {
					prrtMap["commit_id"] = newSha
				}
			}
			if originalCommitId, ok := prrtMap["original_commit_id"].(string); ok {
				if newSha, ok := commitMap[originalCommitId]; ok {
					prrtMap["original_commit_id"] = newSha
				}
			}
		}
	}
	return nil
}

func updateCommitComments(commitMap map[string]string, commitComments *[]interface{}) error {
	for _, cc := range *commitComments {
		if ccMap, ok := cc.(map[string]interface{}); ok {
			// commit_id
			if commitId, ok := ccMap["commit_id"].(string); ok {
				if newSha, ok := commitMap[commitId]; ok {
					ccMap["commit_id"] = newSha
				}
			}
		}
	}
	return nil
}
