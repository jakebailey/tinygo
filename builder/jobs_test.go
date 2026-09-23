package builder

import "testing"

func TestClearJobGraph(t *testing.T) {
	shared := &compileJob{
		run:    func(*compileJob) error { return nil },
		result: "shared",
	}
	left := &compileJob{
		dependencies: []*compileJob{shared},
		run:          func(*compileJob) error { return nil },
		result:       "left",
	}
	right := &compileJob{
		dependencies: []*compileJob{shared},
		run:          func(*compileJob) error { return nil },
		result:       "right",
	}

	clearJobGraph([]*compileJob{left, right})

	for _, job := range []*compileJob{shared, left, right} {
		if job.run != nil {
			t.Errorf("job %q still has a run function", job.result)
		}
		if job.dependencies != nil {
			t.Errorf("job %q still has dependencies", job.result)
		}
	}
}
