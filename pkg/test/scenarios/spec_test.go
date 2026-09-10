package scenarios

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
)

func Test_DeepCopy(t *testing.T) {
	type testStruct struct {
		unexported  string
		StringValue string
		StructValue *testStruct
	}
	tests := []struct {
		name string
		in   testStruct
		want testStruct
	}{
		{
			name: "copy all exported fields by value",
			in: testStruct{
				unexported:  "unexported",
				StringValue: "some value",
				StructValue: &testStruct{
					StringValue: "some other value",
					StructValue: &testStruct{},
				},
			},
			want: testStruct{
				unexported:  "",
				StringValue: "some value",
				StructValue: &testStruct{
					StringValue: "some other value",
					StructValue: &testStruct{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeepCopy(tt.in)
			require.NoError(t, err)
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(testStruct{})); diff != "" {
				t.Errorf("deepCopy() diff = %s", diff)
			}
			in := tt.in
			if got == in {
				t.Errorf("deepCopy() copied struct by reference")
			}
			if got.unexported == in.unexported {
				t.Errorf("deepCopy() copied unexported value")
			}
			if got.StructValue == in.StructValue {
				t.Errorf("deepCopy() copied struct value by reference")
			}
		})
	}
}
