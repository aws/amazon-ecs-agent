package dependencygraph

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
)

func runningContainer(name string, links, volumes []string) *api.Container {
	return &api.Container{
		Name:          name,
		Links:         links,
		VolumesFrom:   volumes,
		DesiredStatus: api.ContainerRunning,
	}
}
func createdContainer(name string, links, volumes []string) *api.Container {
	return &api.Container{
		Name:          name,
		Links:         links,
		VolumesFrom:   volumes,
		DesiredStatus: api.ContainerCreated,
	}
}

func TestValidDependencies(t *testing.T) {
	// Empty task
	task := &api.Task{}
	resolveable := ValidDependencies(task)
	if !resolveable {
		t.Error("The zero dependency graph should resolve")
	}

	task = &api.Task{
		Containers: []*api.Container{
			&api.Container{
				Name:          "redis",
				DesiredStatus: api.ContainerRunning,
			},
		},
	}
	resolveable = ValidDependencies(task)
	if !resolveable {
		t.Error("One container should resolve trivially")
	}

	// Webserver stack
	php := runningContainer("php", []string{"db"}, []string{})
	db := runningContainer("db", []string{}, []string{"dbdatavolume"})
	dbdata := createdContainer("dbdatavolume", []string{}, []string{})
	webserver := runningContainer("webserver", []string{"php"}, []string{"htmldata"})
	htmldata := runningContainer("htmldata", []string{}, []string{"sharedcssfiles"})
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{})

	task = &api.Task{
		Containers: []*api.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	resolveable = ValidDependencies(task)
	if !resolveable {
		t.Error("The webserver group should resolve just fine")
	}

	// Unresolveable: cycle
	task = &api.Task{
		Containers: []*api.Container{
			runningContainer("a", []string{"b"}, []string{}),
			runningContainer("b", []string{"a"}, []string{}),
		},
	}
	resolveable = ValidDependencies(task)
	if resolveable {
		t.Error("Cycle should not be resolveable")
	}
	// Unresolveable, reference doesn't exist
	task = &api.Task{
		Containers: []*api.Container{
			runningContainer("php", []string{"db"}, []string{}),
		},
	}
	resolveable = ValidDependencies(task)
	if resolveable {
		t.Error("Nonexistent reference shouldn't resolve")
	}
}

func TestDependenciesAreResolved(t *testing.T) {
	task := &api.Task{
		Containers: []*api.Container{
			&api.Container{
				Name:          "redis",
				DesiredStatus: api.ContainerRunning,
			},
		},
	}
	resolved := DependenciesAreResolved(task.Containers[0], task.Containers)
	if !resolved {
		t.Error("One container should resolve trivially")
	}

	// Webserver stack
	php := runningContainer("php", []string{"db"}, []string{})
	db := runningContainer("db", []string{}, []string{"dbdatavolume"})
	dbdata := createdContainer("dbdatavolume", []string{}, []string{})
	webserver := runningContainer("webserver", []string{"php"}, []string{"htmldata"})
	htmldata := runningContainer("htmldata", []string{}, []string{"sharedcssfiles"})
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{})

	task = &api.Task{
		Containers: []*api.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	resolved = DependenciesAreResolved(php, task.Containers)
	if resolved {
		t.Error("Shouldn't be resolved; db isn't running")
	}
	resolved = DependenciesAreResolved(db, task.Containers)
	if resolved {
		t.Error("Shouldn't be resolved; dbdatavolume isn't created")
	}
	resolved = DependenciesAreResolved(dbdata, task.Containers)
	if !resolved {
		t.Error("data volume with no deps should resolve")
	}
	dbdata.KnownStatus = api.ContainerCreated

	resolved = DependenciesAreResolved(php, task.Containers)
	if resolved {
		t.Error("Php shouldn't run, db is not created")
	}
	db.KnownStatus = api.ContainerCreated
	resolved = DependenciesAreResolved(php, task.Containers)
	if resolved {
		t.Error("Php shouldn't run, db is not running")
	}

	resolved = DependenciesAreResolved(db, task.Containers)
	if !resolved {
		t.Error("db should be resolved, dbdata volume is Created")
	}
	db.KnownStatus = api.ContainerRunning

	resolved = DependenciesAreResolved(php, task.Containers)
	if !resolved {
		t.Error("Php should resolve")
	}
}
