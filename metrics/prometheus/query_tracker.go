// Copyright © 2025 Meroxa, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus

import (
	"context"
)

// SequentialQueryTracker is a query tracker that allows only one query to be
// active at a time.
type SequentialQueryTracker chan struct{}

func NewSequentialQueryTracker() SequentialQueryTracker {
	return make(SequentialQueryTracker, 1)
}

func (s SequentialQueryTracker) GetMaxConcurrent() int {
	return 1
}
func (s SequentialQueryTracker) Delete(int)   { <-s }
func (s SequentialQueryTracker) Close() error { return nil }

func (s SequentialQueryTracker) Insert(ctx context.Context, _ string) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case s <- struct{}{}:
		return 1, nil
	}
}
