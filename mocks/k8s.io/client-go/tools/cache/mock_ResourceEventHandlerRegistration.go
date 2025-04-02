// Code generated by mockery v2.52.2. DO NOT EDIT.

package cache

import mock "github.com/stretchr/testify/mock"

// MockResourceEventHandlerRegistration is an autogenerated mock type for the ResourceEventHandlerRegistration type
type MockResourceEventHandlerRegistration struct {
	mock.Mock
}

type MockResourceEventHandlerRegistration_Expecter struct {
	mock *mock.Mock
}

func (_m *MockResourceEventHandlerRegistration) EXPECT() *MockResourceEventHandlerRegistration_Expecter {
	return &MockResourceEventHandlerRegistration_Expecter{mock: &_m.Mock}
}

// HasSynced provides a mock function with no fields
func (_m *MockResourceEventHandlerRegistration) HasSynced() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for HasSynced")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// MockResourceEventHandlerRegistration_HasSynced_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'HasSynced'
type MockResourceEventHandlerRegistration_HasSynced_Call struct {
	*mock.Call
}

// HasSynced is a helper method to define mock.On call
func (_e *MockResourceEventHandlerRegistration_Expecter) HasSynced() *MockResourceEventHandlerRegistration_HasSynced_Call {
	return &MockResourceEventHandlerRegistration_HasSynced_Call{Call: _e.mock.On("HasSynced")}
}

func (_c *MockResourceEventHandlerRegistration_HasSynced_Call) Run(run func()) *MockResourceEventHandlerRegistration_HasSynced_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockResourceEventHandlerRegistration_HasSynced_Call) Return(_a0 bool) *MockResourceEventHandlerRegistration_HasSynced_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockResourceEventHandlerRegistration_HasSynced_Call) RunAndReturn(run func() bool) *MockResourceEventHandlerRegistration_HasSynced_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockResourceEventHandlerRegistration creates a new instance of MockResourceEventHandlerRegistration. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockResourceEventHandlerRegistration(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockResourceEventHandlerRegistration {
	mock := &MockResourceEventHandlerRegistration{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
