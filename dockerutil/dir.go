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

package dockerutil

import (
	"context"
)

// dirCtxKey is used as the key when saving the dir in a context.
type dirCtxKey struct{}

// ContextWithDir wraps ctx and returns a context that contains dir.
func ContextWithDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, dirCtxKey{}, dir)
}

// DirFromContext fetches the record dir from the context. If the
// context does not contain a dir it returns an empty string.
func DirFromContext(ctx context.Context) string {
	dir := ctx.Value(dirCtxKey{})
	if dir != nil {
		return dir.(string)
	}
	return ""
}
