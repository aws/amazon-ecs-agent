// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package data

import (
	apira "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
)

func (c *client) SaveEBSAttachment(ebs *apira.ResourceAttachment) error {
	// id, err := utils.GetEBSAttachmentId(ebs.AttachmentARN)
	// if err != nil {
	// 	return errors.Wrap(err, "failed to generate database id")
	// }
	// return c.db.Batch(func(tx *bolt.Tx) error {
	// 	b := tx.Bucket([]byte(eniAttachmentsBucketName))
	// 	return putObject(b, id, ebs)
	// })
	return nil

}
func (c *client) DeleteEBSAttachment(id string) error {
	return nil
}
func (c *client) GetEBSAttachments() ([]**apira.ResourceAttachment, error) {
	return nil, nil
}
