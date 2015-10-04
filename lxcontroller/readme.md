#Lxcontroller
Lxcontroller is a very simple api to provide a simpler and easier approach to creating and performing operations on lxc containers. It aims to simplify the API calls and provide a nice scaffold for container creation,managment and generation

##Install

    go get github.com/influx6/lxcontroller


##Example

```

	name := "verhoef"
	xname := "vandroff"
	killer := make(chan struct{})
	wait := new(sync.WaitGroup)

  //create a profile for the controller,it has the profile for cloning and creating
  //TemplateProfile lets you use the default clone profile and add a custom creation profile
	prof := TemplateProfile(lxc.Directory, "download", "ubuntu", "trusty", "amd64")

  //crate your container
	con2, err := NewController(name, prof)

  //clone it
	con, err2 := con2.Clone(xname)


	go func() {
		<-killer

    //you can drop i.e stop
		err := con.Drop()
    //or Freeze
    err := con.DropFreezed()
    //or Just Release from lxc acquisition while leaving it running in the background
    err := con.DropUnfreezed

		wait.Done()
	}()

	wait.Add(1)
	err = con.Dial()
	checkTestError(t, err, fmt.Sprintf("Dialing Cloned Controller %s", xname))
	Debug.Printf("Success Dialing! Done!")
	close(killer)
	wait.Wait()

```
