package ec2

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/create"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/names"
)

func ResourceInstanceState() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceInstanceStateCreate,
		ReadWithoutTimeout:   resourceInstanceStateRead,
		UpdateWithoutTimeout: resourceInstanceStateUpdate,
		DeleteWithoutTimeout: resourceInstanceStateDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"instance_id": {
				Type:     schema.TypeString,
				ForceNew: true,
				Required: true,
			},
			"state": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringInSlice([]string{ec2.InstanceStateNameRunning, ec2.InstanceStateNameStopped}, false),
			},
			"force": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

func resourceInstanceStateCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).EC2Conn()
	instanceId := d.Get("instance_id").(string)
	instance, instanceErr := FindInstanceByID(conn, instanceId)

	if instanceErr != nil {
		return create.DiagError(names.EC2, create.ErrActionReading, ResInstance, instanceId, instanceErr)
	}

	err := UpdateInstanceState(ctx, conn, instanceId, aws.StringValue(instance.State.Name), d.Get("state").(string), d.Get("force").(bool))

	if err != nil {
		return err
	}

	d.SetId(d.Get("instance_id").(string))

	return resourceInstanceStateRead(ctx, d, meta)

}

func resourceInstanceStateRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).EC2Conn()

	state, err := FindInstanceStateById(ctx, conn, d.Id())

	if !d.IsNewResource() && tfresource.NotFound(err) {
		create.LogNotFoundRemoveState(names.EC2, create.ErrActionReading, ResInstanceState, d.Id())
		d.SetId("")
		return nil
	}

	if err != nil {
		return create.DiagError(names.EC2, create.ErrActionReading, ResInstanceState, d.Id(), err)
	}

	d.Set("instance_id", d.Id())
	d.Set("state", state.Name)
	d.Set("force", d.Get("force").(bool))

	return nil
}

func resourceInstanceStateUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).EC2Conn()

	if d.HasChange("state") {
		o, n := d.GetChange("state")
		err := UpdateInstanceState(ctx, conn, d.Id(), o.(string), n.(string), d.Get("force").(bool))

		if err != nil {
			return err
		}
	}

	return resourceInstanceStateRead(ctx, d, meta)
}

func resourceInstanceStateDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] %s %s deleting an aws_instance_state resource only stops managing instance state, The Instance is left in its current state.: %s", names.EC2, ResInstanceState, d.Id())

	return nil
}

func UpdateInstanceState(ctx context.Context, conn *ec2.EC2, id string, currentState string, configuredState string, force bool) diag.Diagnostics {
	if currentState == configuredState {
		return nil
	}

	if configuredState == "running" {
		if err := StopInstanceWithContext(ctx, conn, id, force, InstanceStopTimeout); err != nil {
			return err
		}
	}

	if configuredState == "stopped" {
		if _, err := WaitInstanceStartedWithContext(ctx, conn, id, InstanceStartTimeout); err != nil {
			return create.DiagError(names.EC2, "starting Instance", ResInstance, id, err)
		}
	}

	return nil
}

func StopInstanceWithContext(ctx context.Context, conn *ec2.EC2, id string, force bool, timeout time.Duration) diag.Diagnostics {
	log.Printf("[INFO] Stopping EC2 Instance: %s, force: %t", id, force)
	_, err := conn.StopInstancesWithContext(ctx, &ec2.StopInstancesInput{
		InstanceIds: aws.StringSlice([]string{id}),
		Force:       aws.Bool(force),
	})

	if err != nil {
		return create.DiagError(names.EC2, "stopping Instance", ResInstance, id, err)
	}

	if _, err := WaitInstanceStoppedWithContext(ctx, conn, id, timeout); err != nil {
		return create.DiagError(names.EC2, "waiting for instance to stop", ResInstance, id, err)
	}

	return nil
}
